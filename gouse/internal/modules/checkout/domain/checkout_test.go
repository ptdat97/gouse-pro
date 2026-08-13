package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
)

// 14:00 — thời điểm khách bấm "Thanh toán" trong ví dụ ở mục 5 của đặc tả.
var testNow = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func bp(t *testing.T, v int32) types.BasisPoints {
	t.Helper()
	b, err := types.NewBasisPoints(v)
	if err != nil {
		t.Fatalf("NewBasisPoints(%d): %v", v, err)
	}
	return b
}

func newLine(t *testing.T, price int64, qty int) *domain.Line {
	t.Helper()
	l, err := domain.NewLine(domain.NewLineParams{
		OfferID:        ids.MustNew(ids.PrefixOffer),
		SKUID:          ids.MustNew(ids.PrefixSKU),
		SellerID:       ids.MustNew(ids.PrefixSeller),
		ProductName:    "Áo sơ mi linen Oxford",
		UnitPrice:      vnd(price),
		Quantity:       qty,
		CommissionRate: bp(t, 1000),
		// Đã giữ được hàng: quy tắc 1 bắt buộc điều này trước khi checkout.
		ReservationID:   ids.MustNew(ids.PrefixReservation),
		InventoryItemID: ids.MustNew(ids.PrefixInventoryItem),
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewLine: %v", err)
	}
	return l
}

func newCheckout(t *testing.T, lines ...*domain.Line) *domain.Checkout {
	t.Helper()
	if len(lines) == 0 {
		lines = []*domain.Line{newLine(t, 299000, 1)}
	}
	c, err := domain.NewCheckout(domain.NewCheckoutParams{
		CartID:     ids.MustNew(ids.PrefixCart),
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Currency:   money.VND,
		Lines:      lines,
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewCheckout: %v", err)
	}
	return c
}

func testAddress() domain.Address {
	return domain.Address{
		RecipientName: "Nguyễn Văn A",
		Phone:         "0900000000",
		StreetAddress: "12 Lý Thường Kiệt",
		Province:      "Hà Nội",
	}
}

// QUY TẮC 2: ĐÓNG BĂNG GIÁ — đúng tình huống ở mục 5 của đặc tả.
//
//	14:00 — Khách bắt đầu checkout, áo giá 299.000đ
//	14:05 — Seller đổi giá thành 350.000đ
//	14:10 — Khách hoàn tất thanh toán
//
//	Không đóng băng: khách thấy 299.000đ nhưng bị trừ 350.000đ
//	Đóng băng:       khách trả đúng 299.000đ như đã thấy
func TestDongBangGiaTaiThoiDiemBatDauCheckout(t *testing.T) {
	// 14:00 — bắt đầu checkout với giá 299.000đ.
	line := newLine(t, 299000, 1)
	c := newCheckout(t, line)

	if c.Total().Amount() != 299000 {
		t.Fatalf("tổng ban đầu = %v, mong 299000", c.Total())
	}

	// 14:05 — seller đổi giá thành 350.000đ.
	//
	// Mô phỏng bằng cách tạo dòng MỚI với giá mới: dòng CŨ trong phiên
	// phải không bị ảnh hưởng. Không có setter nào đổi được giá của dòng
	// đã tạo — đó là cách đóng băng được cưỡng chế.
	newPrice := newLine(t, 350000, 1)
	if newPrice.UnitPrice().Amount() != 350000 {
		t.Fatalf("giá mới = %v, mong 350000", newPrice.UnitPrice())
	}

	// 14:10 — khách hoàn tất. Con số phải là con số khách ĐÃ THẤY.
	at1410 := testNow.Add(10 * time.Minute)
	if err := c.SetShippingAddress(testAddress(), at1410); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if err := c.MarkPendingPayment(at1410); err != nil {
		t.Fatalf("MarkPendingPayment: %v", err)
	}

	if c.Total().Amount() != 299000 {
		t.Errorf("tổng khi thanh toán = %v, mong 299000 — khách bị trừ tiền "+
			"khác con số đã thấy", c.Total())
	}
	if c.Lines()[0].UnitPrice().Amount() != 299000 {
		t.Errorf("đơn giá = %v, mong giữ nguyên 299000", c.Lines()[0].UnitPrice())
	}
}

// QUY TẮC 1: BẮT BUỘC GIỮ TỒN KHO trước khi cho checkout.
//
// Đây là khác biệt cốt lõi với giỏ hàng: phiên này KHÓA hàng thật.
func TestMoiDongPhaiGiuDuocHang(t *testing.T) {
	withStock := newLine(t, 299000, 1)
	if !withStock.HasStock() {
		t.Error("dòng có mã giữ hàng phải báo HasStock")
	}

	noStock, err := domain.NewLine(domain.NewLineParams{
		OfferID:        ids.MustNew(ids.PrefixOffer),
		SellerID:       ids.MustNew(ids.PrefixSeller),
		ProductName:    "Áo sơ mi",
		UnitPrice:      vnd(299000),
		Quantity:       1,
		CommissionRate: bp(t, 1000),
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("NewLine: %v", err)
	}
	if noStock.HasStock() {
		t.Error("dòng không có mã giữ hàng KHÔNG được báo HasStock — bán " +
			"thứ không có là lỗi tệ nhất của luồng này")
	}
}

// PHIÊN SỐNG NGẮN — 15 phút, khác hẳn 30 ngày của giỏ.
//
// Lý do khác nhau là điều duy nhất cần nhớ: phiên này đang KHÓA hàng.
func TestPhienSongNganHonGioRatNhieu(t *testing.T) {
	c := newCheckout(t)

	if got := c.ExpiresAt().Sub(testNow); got != domain.DefaultTTL {
		t.Errorf("thời hạn = %v, mong %v", got, domain.DefaultTTL)
	}
	if domain.DefaultTTL > time.Hour {
		t.Error("phiên thanh toán phải ngắn — nó đang khóa hàng, và hàng bị " +
			"khóa lâu là hàng không bán được cho ai")
	}

	if c.IsExpired(testNow.Add(14 * time.Minute)) {
		t.Error("phiên không được hết hạn ở phút thứ 14")
	}
	if !c.IsExpired(testNow.Add(16 * time.Minute)) {
		t.Error("phiên phải hết hạn sau 15 phút")
	}

	// Thời gian còn lại để client hiển thị đồng hồ đếm ngược.
	if got := c.TimeLeft(testNow.Add(5 * time.Minute)); got != 10*time.Minute {
		t.Errorf("thời gian còn lại = %v, mong 10m", got)
	}
	if got := c.TimeLeft(testNow.Add(20 * time.Minute)); got != 0 {
		t.Errorf("thời gian còn lại sau khi hết hạn = %v, mong 0", got)
	}
}

// HẾT HẠN THEO ĐỒNG HỒ chặn mọi thao tác, KHÔNG ĐỢI tiến trình nền.
//
// Tiến trình nền chạy theo chu kỳ, nên luôn có khoảng trống giữa "hết hạn
// thật" và "được đánh dấu EXPIRED". Trong khoảng đó phiên vẫn mang trạng
// thái STARTED, và nếu chỉ nhìn trạng thái thì khách vẫn thanh toán được
// một phiên mà hàng có thể đã bị nhả.
func TestQuaHanTheoDongHoChanNgayKhongDoiTienTrinhNen(t *testing.T) {
	c := newCheckout(t)
	late := testNow.Add(20 * time.Minute)

	// Trạng thái vẫn là STARTED — chưa ai đánh dấu hết hạn.
	if c.Status() != domain.StatusStarted {
		t.Fatalf("trạng thái = %q, mong STARTED", c.Status())
	}

	for _, tc := range []struct {
		ten string
		err error
	}{
		{"đặt địa chỉ", c.SetShippingAddress(testAddress(), late)},
		{"đặt phí ship", c.SetShipping("giao nhanh", vnd(30000), late)},
		{"áp mã giảm giá", c.ApplyDiscount("THUDONG20", vnd(50000), late)},
		{"chuyển chờ thanh toán", c.MarkPendingPayment(late)},
	} {
		if !errors.Is(tc.err, domain.ErrExpired) {
			t.Errorf("%s: lỗi = %v, mong ErrExpired", tc.ten, tc.err)
		}
	}
}

// GIA HẠN CÓ THẬT nhưng CÓ GIỚI HẠN.
//
// Có thật vì lý do thật: khách đang chuyển khoản ngân hàng cần thêm thời
// gian, và bắt làm lại từ đầu là mất đơn hàng.
//
// Có giới hạn vì gia hạn vô hạn nghĩa là khóa hàng vô hạn — đúng thứ mà
// việc tách checkout khỏi giỏ sinh ra để tránh.
func TestGiaHanCoGioiHan(t *testing.T) {
	c := newCheckout(t)

	for i := 1; i <= domain.MaxExtends; i++ {
		if err := c.Extend(0, testNow); err != nil {
			t.Fatalf("gia hạn lần %d: %v", i, err)
		}
	}
	if c.ExtendedTimes() != domain.MaxExtends {
		t.Errorf("số lần gia hạn = %d, mong %d", c.ExtendedTimes(), domain.MaxExtends)
	}

	// Lần thứ ba bị chặn.
	if err := c.Extend(0, testNow); !errors.Is(err, domain.ErrTooManyExtends) {
		t.Errorf("lỗi = %v, mong ErrTooManyExtends — gia hạn vô hạn là khóa "+
			"hàng vô hạn", err)
	}

	// Thời hạn đã lùi đúng số phút.
	want := testNow.Add(domain.DefaultTTL).
		Add(time.Duration(domain.MaxExtends) * domain.ExtendDuration)
	if !c.ExpiresAt().Equal(want) {
		t.Errorf("thời hạn = %v, mong %v", c.ExpiresAt(), want)
	}
}

// HẾT HẠN RỒI THÌ KHÔNG GIA HẠN ĐƯỢC.
//
// Hàng có thể đã bị nhả và bán cho người khác. Khách phải bắt đầu phiên
// mới, nơi việc giữ hàng được kiểm tra lại từ đầu — gia hạn một phiên đã
// chết là hứa với khách thứ hệ thống không còn nắm giữ.
func TestHetHanRoiThiKhongGiaHanDuoc(t *testing.T) {
	c := newCheckout(t)
	if err := c.Extend(0, testNow.Add(20*time.Minute)); !errors.Is(err, domain.ErrExpired) {
		t.Errorf("lỗi = %v, mong ErrExpired", err)
	}
}

// PHẢI CÓ ĐỊA CHỈ trước khi chuyển sang chờ thanh toán.
//
// Không có địa chỉ thì không tính được phí ship, và đơn tạo ra không giao
// được — phát hiện lúc đó thì tiền đã thu rồi.
func TestPhaiCoDiaChiTruocKhiChoThanhToan(t *testing.T) {
	c := newCheckout(t)

	if err := c.MarkPendingPayment(testNow); !errors.Is(err, domain.ErrNoAddress) {
		t.Errorf("lỗi = %v, mong ErrNoAddress", err)
	}

	// Địa chỉ thiếu tên người nhận cũng không được.
	partial := domain.Address{StreetAddress: "12 Lý Thường Kiệt"}
	if err := c.SetShippingAddress(partial, testNow); !errors.Is(err, domain.ErrNoAddress) {
		t.Errorf("địa chỉ thiếu tên người nhận: lỗi = %v, mong ErrNoAddress", err)
	}

	if err := c.SetShippingAddress(testAddress(), testNow); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if err := c.MarkPendingPayment(testNow); err != nil {
		t.Errorf("có địa chỉ rồi phải chuyển được: %v", err)
	}
}

// TỔNG TIỀN = hàng + ship + thuế − giảm giá.
//
// Đây là CON SỐ KHÁCH NHÌN THẤY, và nó phải bằng đúng con số vào đơn hàng.
func TestTongTienLaConSoKhachNhinThay(t *testing.T) {
	c := newCheckout(t,
		newLine(t, 299000, 2), // 598.000
		newLine(t, 450000, 1), // 450.000
	)
	if c.Subtotal().Amount() != 1048000 {
		t.Fatalf("tiền hàng = %v, mong 1048000", c.Subtotal())
	}

	if err := c.SetShippingAddress(testAddress(), testNow); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if err := c.SetShipping("giao nhanh", vnd(30000), testNow); err != nil {
		t.Fatalf("SetShipping: %v", err)
	}
	if err := c.ApplyDiscount("THUDONG20", vnd(100000), testNow); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}
	if err := c.SetTax(vnd(0), testNow); err != nil {
		t.Fatalf("SetTax: %v", err)
	}

	// 1.048.000 + 30.000 + 0 − 100.000 = 978.000
	if c.Total().Amount() != 978000 {
		t.Errorf("tổng = %v, mong 978000", c.Total())
	}

	// Gỡ mã giảm giá thì tổng tăng lại.
	if err := c.RemoveDiscount(testNow); err != nil {
		t.Fatalf("RemoveDiscount: %v", err)
	}
	if c.Total().Amount() != 1078000 {
		t.Errorf("tổng sau khi gỡ mã = %v, mong 1078000", c.Total())
	}
	if c.CouponCode() != "" {
		t.Errorf("mã giảm giá = %q, mong rỗng", c.CouponCode())
	}
}

// MỌI MÃ GIỮ HÀNG phải lấy ra được để NHẢ.
//
// Sót một cái là khóa hàng vĩnh viễn cho tới khi có người phát hiện thủ
// công — và không ai đi tìm hàng bị khóa cho tới khi hết hàng bán.
func TestLayDuocMoiMaGiuHangDeNha(t *testing.T) {
	a, b, cLine := newLine(t, 299000, 1), newLine(t, 450000, 1), newLine(t, 199000, 1)
	c := newCheckout(t, a, b, cLine)

	ids_ := c.ReservationIDs()
	if len(ids_) != 3 {
		t.Fatalf("số mã giữ hàng = %d, mong 3", len(ids_))
	}

	seen := map[ids.ID]bool{}
	for _, id := range ids_ {
		seen[id] = true
	}
	for _, l := range []*domain.Line{a, b, cLine} {
		if !seen[l.ReservationID()] {
			t.Errorf("thiếu mã giữ hàng %s — hàng này sẽ bị khóa vĩnh viễn",
				l.ReservationID())
		}
	}
}

// PHIÊN CÒN GIỮ HÀNG hay không quyết định khi nào phải nhả.
func TestBietDuocKhiNaoPhienConGiuHang(t *testing.T) {
	for _, tc := range []struct {
		status domain.Status
		giu    bool
	}{
		{domain.StatusStarted, true},
		{domain.StatusPendingPayment, true},
		{domain.StatusCompleted, false},
		{domain.StatusCancelled, false},
		{domain.StatusExpired, false},
	} {
		if got := tc.status.IsHoldingStock(); got != tc.giu {
			t.Errorf("%s.IsHoldingStock() = %v, mong %v", tc.status, got, tc.giu)
		}
	}
}

// QUY TẮC 6: KHÁCH VÃNG LAI được checkout, nhưng phải liên hệ được.
func TestKhachVangLaiCheckoutDuoc(t *testing.T) {
	c, err := domain.NewCheckout(domain.NewCheckoutParams{
		CartID:     ids.MustNew(ids.PrefixCart),
		GuestEmail: "khach@example.com",
		GuestPhone: "0900000000",
		Currency:   money.VND,
		Lines:      []*domain.Line{newLine(t, 299000, 1)},
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("khách vãng lai phải checkout được: %v", err)
	}
	if !c.IsGuest() {
		t.Error("phiên không có customerID phải là phiên khách vãng lai")
	}

	// Không có cả customerID lẫn email → bị chặn: phiên không liên hệ được
	// là đơn không giao được.
	_, err = domain.NewCheckout(domain.NewCheckoutParams{
		CartID:   ids.MustNew(ids.PrefixCart),
		Currency: money.VND,
		Lines:    []*domain.Line{newLine(t, 299000, 1)},
		Now:      testNow,
	})
	if !errors.Is(err, domain.ErrNoCustomer) {
		t.Errorf("lỗi = %v, mong ErrNoCustomer", err)
	}
}

// HOÀN TẤT RỒI thì không hoàn tất lần nữa — nền tảng của idempotency.
func TestHoanTatHaiLanBiChan(t *testing.T) {
	c := newCheckout(t)
	orderID := ids.MustNew(ids.PrefixOrder)

	if err := c.Complete(orderID, "key-1", testNow); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if c.Status() != domain.StatusCompleted {
		t.Errorf("trạng thái = %q, mong COMPLETED", c.Status())
	}
	if c.OrderID() != orderID {
		t.Errorf("mã đơn = %q, mong %q", c.OrderID(), orderID)
	}

	// Lần thứ hai bị chặn: tầng application dùng lỗi này để trả đơn CŨ
	// thay vì tạo đơn thứ hai.
	if err := c.Complete(ids.MustNew(ids.PrefixOrder), "key-1", testNow); !errors.Is(err, domain.ErrAlreadyComplete) {
		t.Errorf("lỗi = %v, mong ErrAlreadyComplete", err)
	}
	// Mã đơn KHÔNG bị ghi đè.
	if c.OrderID() != orderID {
		t.Errorf("mã đơn sau lần gọi thứ hai = %q, mong giữ nguyên %q",
			c.OrderID(), orderID)
	}
}

// PHIÊN ĐÃ KẾT THÚC thì không sửa được nữa.
func TestPhienDaKetThucThiKhoaLai(t *testing.T) {
	for _, tc := range []struct {
		ten  string
		kill func(*domain.Checkout) error
	}{
		{"đã hủy", func(c *domain.Checkout) error { return c.Cancel(testNow) }},
		{"đã hết hạn", func(c *domain.Checkout) error { return c.MarkExpired(testNow) }},
		{"đã hoàn tất", func(c *domain.Checkout) error {
			return c.Complete(ids.MustNew(ids.PrefixOrder), "k", testNow)
		}},
	} {
		t.Run(tc.ten, func(t *testing.T) {
			c := newCheckout(t)
			if err := tc.kill(c); err != nil {
				t.Fatalf("kết thúc phiên: %v", err)
			}

			if err := c.SetShippingAddress(testAddress(), testNow); !errors.Is(err, domain.ErrInvalidStatus) {
				t.Errorf("đặt địa chỉ: lỗi = %v, mong ErrInvalidStatus", err)
			}
			if err := c.Extend(0, testNow); !errors.Is(err, domain.ErrInvalidStatus) {
				t.Errorf("gia hạn: lỗi = %v, mong ErrInvalidStatus", err)
			}
			if err := c.Cancel(testNow); !errors.Is(err, domain.ErrInvalidStatus) {
				t.Errorf("hủy lần nữa: lỗi = %v, mong ErrInvalidStatus", err)
			}
		})
	}
}

// NHÓM THEO NGUỒN HÀNG để tính phí ship và hiển thị thời gian giao riêng.
//
// Quy tắc 7: khách cần biết món nào đến trước.
func TestNhomTheoNguonHang(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	mk := func(seller ids.ID) *domain.Line {
		l := newLine(t, 299000, 1)
		return mustLine(t, seller, l)
	}

	c := newCheckout(t, mk(sellerA), mk(sellerB), mk(sellerA))

	if got := c.SellerIDs(); len(got) != 2 {
		t.Errorf("số nguồn hàng = %d, mong 2 (không trùng lặp)", len(got))
	}
	if len(c.Lines()) != 3 {
		t.Errorf("số dòng = %d, mong 3 — phiên không gộp dòng theo seller",
			len(c.Lines()))
	}
}

// mustLine tạo lại dòng với seller chỉ định.
func mustLine(t *testing.T, sellerID ids.ID, src *domain.Line) *domain.Line {
	t.Helper()
	l, err := domain.NewLine(domain.NewLineParams{
		OfferID:         src.OfferID(),
		SKUID:           src.SKUID(),
		SellerID:        sellerID,
		ProductName:     src.ProductName(),
		UnitPrice:       src.UnitPrice(),
		Quantity:        src.Quantity(),
		CommissionRate:  src.CommissionRate(),
		ReservationID:   src.ReservationID(),
		InventoryItemID: src.InventoryItemID(),
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewLine: %v", err)
	}
	return l
}
