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
	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
	customerhttp "github.com/fashion-commerce/platform/internal/modules/customer/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

// ---------------------------------------------------------------- Bản giả
//
// Quy tắc R8 của archcheck cấm tầng interfaces import infrastructure, kể cả
// trong test. Đó là ràng buộc ĐÚNG ở đây: test tầng này nói về chuyện của
// HTTP — ai được xem gì, mã trạng thái, hình dạng JSON.
//
// Hai bất biến dựa vào DATABASE — cờ notify ghi xuống được, và thêm lại món
// đã thích không tạo bản sao — được kiểm chứng ở module_test.go với
// PostgreSQL thật. Kiểm ở đây bằng bản giả chỉ là kiểm chính bản giả.

type fakeCustomers struct{ byID map[ids.ID]*domain.Customer }

func (f *fakeCustomers) Save(_ context.Context, c *domain.Customer) error {
	f.byID[c.ID()] = c
	return nil
}

func (f *fakeCustomers) Update(_ context.Context, c *domain.Customer) error {
	f.byID[c.ID()] = c
	return nil
}

func (f *fakeCustomers) FindByID(_ context.Context, id ids.ID) (*domain.Customer, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeCustomers) FindByEmail(_ context.Context, email string) (*domain.Customer, error) {
	for _, c := range f.byID {
		if c.Email() == domain.NormalizeEmail(email) {
			return c, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeCustomers) FindByUserID(_ context.Context, _ ids.ID) (*domain.Customer, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeCustomers) FindManyByIDs(
	_ context.Context, _ []ids.ID,
) (map[ids.ID]*domain.Customer, error) {
	return map[ids.ID]*domain.Customer{}, nil
}

type fakeAddresses struct{ byCustomer map[ids.ID][]*domain.Address }

func (f *fakeAddresses) Save(_ context.Context, a *domain.Address) error {
	f.byCustomer[a.CustomerID()] = append(f.byCustomer[a.CustomerID()], a)
	return nil
}

func (f *fakeAddresses) Update(_ context.Context, _ *domain.Address) error { return nil }

func (f *fakeAddresses) FindByID(_ context.Context, id ids.ID) (*domain.Address, error) {
	for _, list := range f.byCustomer {
		for _, a := range list {
			if a.ID() == id {
				return a, nil
			}
		}
	}
	return nil, domain.ErrAddressNotFound
}

func (f *fakeAddresses) ListByCustomer(
	_ context.Context, customerID ids.ID,
) ([]*domain.Address, error) {
	return f.byCustomer[customerID], nil
}

func (f *fakeAddresses) FindDefault(
	_ context.Context, customerID ids.ID,
) (*domain.Address, error) {
	for _, a := range f.byCustomer[customerID] {
		if a.IsDefault() {
			return a, nil
		}
	}
	return nil, domain.ErrAddressNotFound
}

func (f *fakeAddresses) ClearDefault(_ context.Context, _ ids.ID, _ time.Time) error {
	return nil
}

func (f *fakeAddresses) SetDefault(_ context.Context, _, _ ids.ID, _ time.Time) error {
	return nil
}

func (f *fakeAddresses) Delete(_ context.Context, _ ids.ID, _ time.Time) error { return nil }

type fakeWishlists struct {
	lists map[ids.ID]*domain.Wishlist
	items map[ids.ID][]domain.WishlistItem
}

func (f *fakeWishlists) Save(_ context.Context, w *domain.Wishlist) error {
	f.lists[w.CustomerID()] = w
	return nil
}

func (f *fakeWishlists) FindDefault(
	_ context.Context, customerID ids.ID,
) (*domain.Wishlist, error) {
	w, ok := f.lists[customerID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreWishlist(domain.RestoreWishlistParams{
		ID: w.ID(), CustomerID: w.CustomerID(), Name: w.Name(),
		IsDefault: true, Items: f.items[w.ID()],
	}), nil
}

func (f *fakeWishlists) AddItem(
	_ context.Context, wishlistID ids.ID, item domain.WishlistItem,
) (bool, error) {
	for _, existing := range f.items[wishlistID] {
		if existing.ProductID == item.ProductID && existing.VariantID == item.VariantID {
			return false, nil
		}
	}
	f.items[wishlistID] = append(f.items[wishlistID], item)
	return true, nil
}

func (f *fakeWishlists) RemoveItem(
	_ context.Context, _, _, _ ids.ID,
) (bool, error) {
	return false, nil
}

func (f *fakeWishlists) CountByProduct(_ context.Context, _ ids.ID) (int, error) {
	return 0, nil
}

// newHandler dựng handler với kho lưu trữ giả.
func newHandler(t *testing.T) (http.Handler, *application.Service) {
	t.Helper()

	svc := application.NewService(application.Deps{
		Customers: &fakeCustomers{byID: map[ids.ID]*domain.Customer{}},
		Addresses: &fakeAddresses{byCustomer: map[ids.ID][]*domain.Address{}},
		Wishlists: &fakeWishlists{
			lists: map[ids.ID]*domain.Wishlist{},
			items: map[ids.ID][]domain.WishlistItem{},
		},
	})

	mux := http.NewServeMux()
	customerhttp.NewHandler(svc,
		slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	return mux, svc
}

// newCustomer tạo một hồ sơ khách hàng thật.
func newCustomer(t *testing.T, svc *application.Service, email string) ids.ID {
	t.Helper()
	c, err := svc.Create(context.Background(), application.CreateInput{
		Email:       email,
		DisplayName: "Nguyễn Văn A",
		Phone:       "+84901234567",
	})
	if err != nil {
		t.Fatalf("tạo khách hàng: %v", err)
	}
	return c.ID()
}

func asCustomer(r *http.Request, id ids.ID) *http.Request {
	return r.WithContext(httpserver.WithShopper(r.Context(),
		httpserver.Shopper{CustomerID: id.String(), SessionID: "ses_test"}))
}

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

func jsonReq(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// ---------------------------------------------------------------- Test

// TestKhachVangLaiKhongVaoDuocTaiKhoan là test quan trọng nhất của tệp này.
//
// Khác cart và checkout — vốn PHẢI chạy cho khách vãng lai — mọi endpoint ở
// đây đọc hoặc ghi dữ liệu cá nhân. Cho khách chưa đăng nhập đi qua nghĩa là
// mọi request ẩn danh cùng trỏ vào một hồ sơ "định danh rỗng".
func TestKhachVangLaiKhongVaoDuocTaiKhoan(t *testing.T) {
	h, _ := newHandler(t)

	cases := []struct {
		method, path string
	}{
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/me/addresses"},
		{"GET", "/api/v1/me/wishlist"},
	}

	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			res, _ := do(t, h, asGuest(httptest.NewRequest(c.method, c.path, nil)))
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("mã trạng thái = %d, muốn 401", res.StatusCode)
			}
		})
	}
}

// TestChiThayHoSoCuaChinhMinh kiểm chứng phạm vi dữ liệu.
//
// Hai khách khác nhau phải nhận hai hồ sơ khác nhau, kể cả khi gọi cùng một
// đường dẫn — vì đường dẫn KHÔNG mang định danh nào cả.
func TestChiThayHoSoCuaChinhMinh(t *testing.T) {
	h, svc := newHandler(t)

	a := newCustomer(t, svc, "a@example.com")
	b := newCustomer(t, svc, "b@example.com")

	_, bodyA := do(t, h, asCustomer(httptest.NewRequest("GET", "/api/v1/me", nil), a))
	_, bodyB := do(t, h, asCustomer(httptest.NewRequest("GET", "/api/v1/me", nil), b))

	idA := bodyA["customer"].(map[string]any)["id"]
	idB := bodyB["customer"].(map[string]any)["id"]
	if idA == idB {
		t.Fatalf("hai khách nhận cùng một hồ sơ: %v", idA)
	}
	if got := bodyA["customer"].(map[string]any)["email"]; got != "a@example.com" {
		t.Errorf("email = %v, muốn a@example.com", got)
	}
}

// TestKhongDoiDuocEmailQuaHoSo kiểm chứng email KHÔNG sửa được ở đây.
//
// Đổi email là đổi DANH TÍNH, không phải sửa hồ sơ: nó cần xác minh quyền
// sở hữu địa chỉ mới. Cho đi qua nghĩa là đổi email tài khoản người khác
// thành của mình là chiếm được tài khoản đó.
//
// DisallowUnknownFields biến việc này thành lỗi 400 thay vì bỏ qua im lặng
// — im lặng thì client tưởng đã đổi được.
func TestKhongDoiDuocEmailQuaHoSo(t *testing.T) {
	h, svc := newHandler(t)
	id := newCustomer(t, svc, "a@example.com")

	r := jsonReq("PATCH", "/api/v1/me", map[string]any{"email": "keTomo@example.com"})
	res, _ := do(t, h, asCustomer(r, id))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("mã trạng thái = %d, muốn 400", res.StatusCode)
	}

	// Và email PHẢI còn nguyên: mã trạng thái đúng mà dữ liệu vẫn đổi thì
	// test chỉ kiểm chứng thông báo, không kiểm chứng hậu quả.
	c, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("đọc lại hồ sơ: %v", err)
	}
	if c.Email() != "a@example.com" {
		t.Errorf("email = %q, đã bị đổi", c.Email())
	}
}

// TestSuaHoSoLuuXuongDatabase kiểm chứng vòng ghi–đọc.
func TestSuaHoSoLuuXuongDatabase(t *testing.T) {
	h, svc := newHandler(t)
	id := newCustomer(t, svc, "a@example.com")

	r := jsonReq("PATCH", "/api/v1/me", map[string]any{
		"name": "Trần Thị B", "phone": "+84909999999",
	})
	res, _ := do(t, h, asCustomer(r, id))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200", res.StatusCode)
	}

	c, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("đọc lại hồ sơ: %v", err)
	}
	if c.DisplayName() != "Trần Thị B" {
		t.Errorf("tên = %q, muốn Trần Thị B", c.DisplayName())
	}
}

// TestSoDiaChiRiengTungKhach kiểm chứng địa chỉ không rò sang khách khác.
//
// Sổ địa chỉ chứa nơi ở thật của khách. Rò rỉ nó không phải lỗi hiển thị.
func TestSoDiaChiRiengTungKhach(t *testing.T) {
	h, svc := newHandler(t)
	a := newCustomer(t, svc, "a@example.com")
	b := newCustomer(t, svc, "b@example.com")

	r := jsonReq("POST", "/api/v1/me/addresses", map[string]any{
		"recipient_name": "Nguyễn Văn A",
		"phone":          "+84901234567",
		"street_address": "12 Lê Lợi",
		"ward":           "Bến Nghé",
		"district":       "Quận 1",
		"province":       "TP. Hồ Chí Minh",
		"country_code":   "VN",
		"is_default":     true,
	})
	res, body := do(t, h, asCustomer(r, a))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("thêm địa chỉ: mã trạng thái = %d, thân = %v", res.StatusCode, body)
	}

	// Khách A thấy địa chỉ của mình.
	_, bodyA := do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/me/addresses", nil), a))
	if n := len(bodyA["data"].([]any)); n != 1 {
		t.Fatalf("khách A có %d địa chỉ, muốn 1", n)
	}

	// Khách B KHÔNG thấy gì.
	_, bodyB := do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/me/addresses", nil), b))
	data, ok := bodyB["data"].([]any)
	if !ok {
		t.Fatalf("`data` phải là mảng, kể cả khi rỗng: %#v", bodyB["data"])
	}
	if len(data) != 0 {
		t.Errorf("khách B thấy %d địa chỉ của người khác", len(data))
	}
}

// TestYeuThichTraDungHinhDangJSON kiểm chứng handler ĐỌC được cờ notify từ
// thân request và TRẢ nó ra đúng tên trường của đặc tả.
//
// Việc cờ đó có ghi xuống database hay không được kiểm ở module_test.go với
// PostgreSQL thật — bản giả ở đây sẽ luôn nhớ đúng thứ nó vừa nhận.
func TestYeuThichTraDungHinhDangJSON(t *testing.T) {
	h, svc := newHandler(t)
	id := newCustomer(t, svc, "a@example.com")
	productID := ids.MustNew(ids.PrefixProduct)

	r := jsonReq("POST", "/api/v1/me/wishlist", map[string]any{
		"product_id":            productID.String(),
		"notify_when_available": true,
	})
	res, body := do(t, h, asCustomer(r, id))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("mã trạng thái = %d, thân = %v", res.StatusCode, body)
	}
	if body["added"] != true {
		t.Errorf("added = %v, muốn true", body["added"])
	}

	_, list := do(t, h, asCustomer(
		httptest.NewRequest("GET", "/api/v1/me/wishlist", nil), id))
	items := list["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("có %d món yêu thích, muốn 1", len(items))
	}
	item := items[0].(map[string]any)
	if item["notify_when_available"] != true {
		t.Errorf("notify_when_available = %v, muốn true — cờ bị vứt đi",
			item["notify_when_available"])
	}
	if item["product_id"] != productID.String() {
		t.Errorf("product_id = %v, muốn %v", item["product_id"], productID)
	}
}

// TestThieuProductIDBiTuChoi kiểm chứng trường bắt buộc của đặc tả.
func TestThieuProductIDBiTuChoi(t *testing.T) {
	h, svc := newHandler(t)
	id := newCustomer(t, svc, "a@example.com")

	res, _ := do(t, h, asCustomer(
		jsonReq("POST", "/api/v1/me/wishlist", map[string]any{}), id))
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400", res.StatusCode)
	}
}
