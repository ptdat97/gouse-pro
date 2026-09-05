package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/application"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
	orderhttp "github.com/fashion-commerce/platform/internal/modules/order/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

// fakeRepo là kho lưu trữ trong bộ nhớ.
//
// Quy tắc R8 của archcheck cấm tầng interfaces import infrastructure, kể cả
// trong test. Điều đó giữ cho test ở tầng này nói về CHUYỆN CỦA HTTP —
// quyền xem đơn, mã trạng thái, hình dạng JSON — thay vì chuyện database.
type fakeRepo struct{ orders []*domain.Order }

var _ domain.Repository = (*fakeRepo)(nil)

func (r *fakeRepo) Save(_ context.Context, o *domain.Order) error {
	r.orders = append(r.orders, o)
	return nil
}

func (r *fakeRepo) Update(_ context.Context, _ *domain.Order) error { return nil }

func (r *fakeRepo) FindByID(_ context.Context, id ids.ID) (*domain.Order, error) {
	for _, o := range r.orders {
		if o.ID() == id {
			return o, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) FindByOrderNumber(_ context.Context, n string) (*domain.Order, error) {
	for _, o := range r.orders {
		if o.OrderNumber() == n {
			return o, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) FindByIdempotencyKey(_ context.Context, _ string) (*domain.Order, error) {
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) FindBySourceCheckout(_ context.Context, _ ids.ID) (*domain.Order, error) {
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) ListByCustomer(
	_ context.Context, customerID ids.ID, status domain.Status, limit, offset int,
) ([]*domain.Order, error) {
	var mine []*domain.Order
	for _, o := range r.orders {
		// Lọc trạng thái NGAY ở đây, giống câu SQL thật.
		//
		// Bản giả lọc khác bản thật thì test xanh trên một hành vi không
		// tồn tại — đúng thứ bản giả sinh ra để tránh.
		if o.CustomerID() != customerID {
			continue
		}
		if status != "" && o.Status() != status {
			continue
		}
		mine = append(mine, o)
	}
	if offset >= len(mine) {
		return nil, nil
	}
	mine = mine[offset:]
	if limit > 0 && len(mine) > limit {
		mine = mine[:limit]
	}
	return mine, nil
}

func (r *fakeRepo) List(_ context.Context, _ domain.Filter) ([]*domain.Order, error) {
	return r.orders, nil
}

func (r *fakeRepo) UpdateWithAudit(
	ctx context.Context, o *domain.Order, fn domain.TxFunc,
) error {
	if fn != nil {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return r.Update(ctx, o)
}

// ---------------------------------------------------------------- Dữ liệu

// newOrder tạo một đơn hàng đã đặt.
//
// customerID rỗng nghĩa là đơn của khách VÃNG LAI — khi đó email và số điện
// thoại là cách duy nhất liên hệ, và số điện thoại là cách duy nhất tra đơn.
func newOrder(t *testing.T, customerID ids.ID, guestPhone string) *domain.Order {
	t.Helper()

	rate, err := types.NewBasisPoints(1000)
	if err != nil {
		t.Fatalf("tỷ lệ hoa hồng: %v", err)
	}

	line, err := domain.NewLine(domain.NewLineParams{
		OfferID:            ids.MustNew(ids.PrefixOffer),
		SKUID:              ids.MustNew(ids.PrefixSKU),
		SellerID:           ids.MustNew(ids.PrefixSeller),
		ProductName:        "Áo sơ mi linen Oxford",
		VariantDescription: "Trắng / M",
		UnitPrice:          money.MustNew(299_000, money.VND),
		Quantity:           2,
		CommissionRate:     rate,
	})
	if err != nil {
		t.Fatalf("tạo dòng hàng: %v", err)
	}

	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber: "FC-2026-08-" + string(ids.MustNew(ids.PrefixOrder))[24:],
		CustomerID:  customerID,
		GuestEmail:  "khach@example.com",
		GuestPhone:  guestPhone,
		ShippingAddress: domain.Address{
			RecipientName: "Nguyễn Văn A",
			Phone:         guestPhone,
			StreetAddress: "12 Lê Lợi",
			Province:      "TP. Hồ Chí Minh",
			CountryCode:   "VN",
		},
		Currency:       money.VND,
		ShippingFee:    money.MustNew(30_000, money.VND),
		Lines:          []*domain.Line{line},
		IdempotencyKey: string(ids.MustNew(ids.PrefixOrder)),
		Now:            time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("tạo đơn hàng: %v", err)
	}
	return o
}

// newHandler dựng handler trần.
//
// KHÔNG bọc ResolveShopper: các test dưới đây gắn thẳng danh tính vào
// context để nói rõ ai đang gọi. Bản thân middleware đã được kiểm chứng ở
// cart/interfaces/http — lặp lại ở đây chỉ làm mờ điều đang thử.
func newHandler(t *testing.T) (http.Handler, *fakeRepo) {
	t.Helper()

	repo := &fakeRepo{}
	svc := application.NewService(application.Deps{Orders: repo})

	mux := http.NewServeMux()
	orderhttp.NewCustomerHandler(svc,
		slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	return mux, repo
}

// asCustomer gắn danh tính khách ĐÃ ĐĂNG NHẬP, thay cho việc dựng cả luồng
// đăng nhập.
func asCustomer(r *http.Request, customerID ids.ID) *http.Request {
	return r.WithContext(httpserver.WithShopper(r.Context(),
		httpserver.Shopper{CustomerID: customerID.String(), SessionID: "ses_test"}))
}

// asGuest gắn danh tính khách VÃNG LAI: có phiên, không có hồ sơ khách hàng.
func asGuest(r *http.Request) *http.Request {
	return r.WithContext(httpserver.WithShopper(r.Context(),
		httpserver.Shopper{SessionID: "ses_test"}))
}

func do(t *testing.T, h http.Handler, r *http.Request) (*http.Response, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	res := w.Result()

	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res, decoded
}

// ---------------------------------------------------------------- Test

// TestChiThayDonCuaChinhMinh là test quan trọng nhất của tệp này.
//
// Lịch sử mua hàng chứa tên, địa chỉ và những gì một người đã mua. Rò rỉ nó
// không phải "lỗi hiển thị" — đó là lộ dữ liệu cá nhân của người khác.
func TestChiThayDonCuaChinhMinh(t *testing.T) {
	h, repo := newHandler(t)

	toi := ids.MustNew(ids.PrefixCustomer)
	nguoiKhac := ids.MustNew(ids.PrefixCustomer)
	_ = repo.Save(context.Background(), newOrder(t, toi, "+84901234567"))
	_ = repo.Save(context.Background(), newOrder(t, nguoiKhac, "+84907654321"))
	_ = repo.Save(context.Background(), newOrder(t, nguoiKhac, "+84907654321"))

	res, body := do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/orders", nil), toi))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200", res.StatusCode)
	}

	data := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("trả về %d đơn, muốn 1 — thấy cả đơn của người khác", len(data))
	}
}

// TestChuaDangNhapKhongXemDuocLichSu kiểm chứng đường danh sách đóng với
// khách vãng lai.
//
// Khách vãng lai không có định danh bền vững nào ngoài cookie phiên, và
// cookie phiên đổi mỗi lần đổi thiết bị. Trả danh sách theo cookie nghĩa là
// hai người dùng chung một máy thấy đơn của nhau.
func TestChuaDangNhapKhongXemDuocLichSu(t *testing.T) {
	h, _ := newHandler(t)

	res, _ := do(t, h, asGuest(httptest.NewRequest("GET", "/api/v1/orders", nil)))
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("mã trạng thái = %d, muốn 401", res.StatusCode)
	}
}

// TestKhongXemDuocChiTietDonNguoiKhac kiểm chứng quyền ở đường chi tiết.
//
// Response chứa tên người nhận, số điện thoại và địa chỉ đầy đủ.
func TestKhongXemDuocChiTietDonNguoiKhac(t *testing.T) {
	h, repo := newHandler(t)

	chuDon := ids.MustNew(ids.PrefixCustomer)
	keTomo := ids.MustNew(ids.PrefixCustomer)
	o := newOrder(t, chuDon, "+84901234567")
	_ = repo.Save(context.Background(), o)

	res, _ := do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/orders/"+o.ID().String(), nil), keTomo))

	// 404 chứ không phải 403: mã hiển thị của đơn tăng dần, nên hai mã khác
	// nhau cho "có thật" và "không có" sẽ đếm được số đơn của nền tảng.
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("mã trạng thái = %d, muốn 404", res.StatusCode)
	}
}

// TestKhachVangLaiTraDonBangSoDienThoai kiểm chứng đường tra đơn của khách
// không có tài khoản.
//
// Họ chỉ có mã hiển thị trong email xác nhận, không có mã nội bộ — nên
// đường dẫn phải nhận cả hai dạng.
func TestKhachVangLaiTraDonBangSoDienThoai(t *testing.T) {
	h, repo := newHandler(t)

	o := newOrder(t, "", "+84901234567")
	_ = repo.Save(context.Background(), o)

	// Đúng số điện thoại → xem được, tra bằng MÃ HIỂN THỊ.
	r := asGuest(httptest.NewRequest("GET", "/api/v1/orders/"+o.OrderNumber(), nil))
	r.Header.Set("X-Guest-Phone", "+84901234567")
	res, body := do(t, h, r)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200, thân = %v", res.StatusCode, body)
	}
	if body["order_number"] != o.OrderNumber() {
		t.Errorf("trả về đơn %v, muốn %v", body["order_number"], o.OrderNumber())
	}

	// Sai số điện thoại → không xem được.
	r = asGuest(httptest.NewRequest("GET", "/api/v1/orders/"+o.OrderNumber(), nil))
	r.Header.Set("X-Guest-Phone", "+84900000000")
	if res, _ := do(t, h, r); res.StatusCode != http.StatusNotFound {
		t.Errorf("sai số điện thoại: mã trạng thái = %d, muốn 404", res.StatusCode)
	}

	// Không gửi số điện thoại → không xem được.
	r = asGuest(httptest.NewRequest("GET", "/api/v1/orders/"+o.OrderNumber(), nil))
	if res, _ := do(t, h, r); res.StatusCode != http.StatusNotFound {
		t.Errorf("thiếu số điện thoại: mã trạng thái = %d, muốn 404", res.StatusCode)
	}
}

// TestLyDoHuyPhaiNamTrongDanhSach kiểm chứng danh sách lý do là ĐÓNG.
//
// Năm lý do dẫn tới năm hành động khác nhau của nền tảng. Cho chuỗi tự do
// đi qua nghĩa là không tổng hợp được gì.
func TestLyDoHuyPhaiNamTrongDanhSach(t *testing.T) {
	h, repo := newHandler(t)

	customerID := ids.MustNew(ids.PrefixCustomer)
	o := newOrder(t, customerID, "+84901234567")
	_ = repo.Save(context.Background(), o)

	r := httptest.NewRequest("POST", "/api/v1/orders/"+o.ID().String()+"/cancel",
		bytes.NewBufferString(`{"reason":"KHONG_THICH_NUA"}`))
	r.Header.Set("Content-Type", "application/json")

	res, _ := do(t, h, asCustomer(r, customerID))
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400", res.StatusCode)
	}
}

// TestKhongHuyDuocDonNguoiKhac kiểm chứng quyền ở đường hủy đơn.
//
// Đây là thao tác GHI: đọc nhầm đơn người khác là lộ dữ liệu, hủy nhầm đơn
// người khác là phá hàng hóa họ đang chờ.
func TestKhongHuyDuocDonNguoiKhac(t *testing.T) {
	h, repo := newHandler(t)

	chuDon := ids.MustNew(ids.PrefixCustomer)
	keTomo := ids.MustNew(ids.PrefixCustomer)
	o := newOrder(t, chuDon, "+84901234567")
	_ = repo.Save(context.Background(), o)

	r := httptest.NewRequest("POST", "/api/v1/orders/"+o.ID().String()+"/cancel",
		bytes.NewBufferString(`{"reason":"CHANGED_MIND"}`))
	r.Header.Set("Content-Type", "application/json")

	res, _ := do(t, h, asCustomer(r, keTomo))
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("mã trạng thái = %d, muốn 404", res.StatusCode)
	}

	// Và đơn PHẢI còn nguyên: mã trạng thái đúng mà đơn vẫn bị hủy thì test
	// chỉ kiểm chứng thông báo, không kiểm chứng hậu quả.
	if o.Status() == domain.StatusCancelled {
		t.Error("đơn của người khác đã bị hủy")
	}
}

// TestHuyDonGhiLaiLyDo kiểm chứng lý do được LƯU chứ không chỉ kiểm tra.
//
// Bắt khách chọn một trong năm lý do rồi vứt đi là bắt họ làm việc vô ích.
func TestHuyDonGhiLaiLyDo(t *testing.T) {
	h, repo := newHandler(t)

	customerID := ids.MustNew(ids.PrefixCustomer)
	o := newOrder(t, customerID, "+84901234567")
	_ = repo.Save(context.Background(), o)

	r := httptest.NewRequest("POST", "/api/v1/orders/"+o.ID().String()+"/cancel",
		bytes.NewBufferString(`{"reason":"DELIVERY_TOO_SLOW"}`))
	r.Header.Set("Content-Type", "application/json")

	res, body := do(t, h, asCustomer(r, customerID))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200, thân = %v", res.StatusCode, body)
	}
	if got := body["order"].(map[string]any)["status"]; got != "CANCELLED" {
		t.Errorf("trạng thái = %v, muốn CANCELLED", got)
	}
	if o.CancellationReason() != "DELIVERY_TOO_SLOW" {
		t.Errorf("lý do hủy = %q, muốn DELIVERY_TOO_SLOW — lý do bị vứt đi",
			o.CancellationReason())
	}
}

// TestPhanTrangBaoDungConTrangSau kiểm chứng `has_more`.
//
// Sai trường này thì giao diện hoặc dừng sớm (khách mất lịch sử) hoặc gọi
// mãi một trang rỗng.
func TestPhanTrangBaoDungConTrangSau(t *testing.T) {
	h, repo := newHandler(t)

	customerID := ids.MustNew(ids.PrefixCustomer)
	for i := 0; i < 3; i++ {
		_ = repo.Save(context.Background(), newOrder(t, customerID, "+84901234567"))
	}

	res, body := do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/orders?limit=2", nil), customerID))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200", res.StatusCode)
	}

	if got := len(body["data"].([]any)); got != 2 {
		t.Errorf("trang có %d đơn, muốn 2", got)
	}
	pg := body["pagination"].(map[string]any)
	if pg["has_more"] != true {
		t.Error("has_more = false, muốn true — còn một đơn chưa trả")
	}

	// Trang cuối: hết dữ liệu thì has_more phải tắt.
	_, body = do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/orders?limit=2&cursor=2", nil), customerID))
	if pg := body["pagination"].(map[string]any); pg["has_more"] != false {
		t.Error("has_more = true ở trang cuối")
	}
}
