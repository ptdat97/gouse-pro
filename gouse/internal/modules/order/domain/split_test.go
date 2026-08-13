package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
)

var testNow = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func bp(t *testing.T, v int32) types.BasisPoints {
	t.Helper()
	b, err := types.NewBasisPoints(v)
	if err != nil {
		t.Fatalf("NewBasisPoints: %v", err)
	}
	return b
}

func newLine(t *testing.T, sellerID ids.ID, price int64, qty int, rate int32) *domain.Line {
	t.Helper()
	l, err := domain.NewLine(domain.NewLineParams{
		OfferID:            ids.MustNew(ids.PrefixOffer),
		SKUID:              ids.MustNew(ids.PrefixSKU),
		SellerID:           sellerID,
		ProductName:        "Áo sơ mi linen",
		VariantDescription: "Trắng / M",
		UnitPrice:          vnd(price),
		Quantity:           qty,
		CommissionRate:     bp(t, rate),
		Now:                testNow,
	})
	if err != nil {
		t.Fatalf("NewLine: %v", err)
	}
	return l
}

func newOrder(t *testing.T, lines ...*domain.Line) *domain.Order {
	t.Helper()
	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber:    "FC-2026-08-001234",
		CustomerID:     ids.MustNew(ids.PrefixCustomer),
		Lines:          lines,
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return o
}

// TÁCH ĐƠN THEO NGUỒN HÀNG — quyết định cốt lõi của ADR-0007.
//
//	Giỏ hàng:
//	├── Áo own brand   (kho nền tảng, Hà Nội)
//	├── Giày Seller A  (kho seller A, TP.HCM)
//	└── Túi Seller B   (kho seller B, Đà Nẵng)
//
//	Ba món KHÔNG THỂ đóng chung một gói.
func TestTachDonThanhBaFulfillmentOrder(t *testing.T) {
	ownBrand := ids.MustNew(ids.PrefixSeller)
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	o := newOrder(t,
		newLine(t, ownBrand, 300000, 1, 0),   // own brand: hoa hồng 0
		newLine(t, sellerA, 500000, 1, 1000), // seller A: 10%
		newLine(t, sellerB, 450000, 1, 1000), // seller B: 10%
	)

	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	if len(fos) != 3 {
		t.Fatalf("số đơn thực hiện = %d, mong 3 — mỗi nguồn hàng một đơn", len(fos))
	}

	// Mã FO đánh theo thứ tự: seller thấy mã của mình mà không cần biết
	// có bao nhiêu seller khác.
	wantNumbers := []string{
		"FC-2026-08-001234-A", "FC-2026-08-001234-B", "FC-2026-08-001234-C",
	}
	for i, fo := range fos {
		if fo.FONumber() != wantNumbers[i] {
			t.Errorf("mã FO %d = %q, mong %q", i, fo.FONumber(), wantNumbers[i])
		}
		if fo.OrderID() != o.ID() {
			t.Error("đơn thực hiện không trỏ về đơn gốc")
		}
		if fo.Status() != domain.FOPending {
			t.Errorf("trạng thái = %q, mong PENDING", fo.Status())
		}
	}

	// Own brand cũng được tách như seller bình thường — nó là seller nội bộ.
	if fos[0].SellerID() != ownBrand {
		t.Error("own brand phải được tách thành một đơn thực hiện riêng")
	}
	// Own brand hoa hồng 0 → toàn bộ tiền là của nền tảng.
	if fos[0].SellerPayable().Amount() != 300000 {
		t.Errorf("own brand payable = %v, mong 300000", fos[0].SellerPayable())
	}
	// Seller A: 500.000 − 10% = 450.000.
	if fos[1].SellerPayable().Amount() != 450000 {
		t.Errorf("seller A payable = %v, mong 450000", fos[1].SellerPayable())
	}
}

// Nhiều dòng CÙNG seller gộp vào MỘT đơn thực hiện: chúng đóng chung được.
func TestNhieuDongCungSellerGopMotDon(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	o := newOrder(t,
		newLine(t, sellerA, 100000, 2, 1000),
		newLine(t, sellerB, 200000, 1, 1000),
		newLine(t, sellerA, 300000, 1, 1000), // lại seller A
	)

	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}
	if len(fos) != 2 {
		t.Fatalf("số đơn thực hiện = %d, mong 2", len(fos))
	}

	// Đơn của seller A có HAI dòng, tổng 200.000 + 300.000 = 500.000.
	if len(fos[0].LineIDs()) != 2 {
		t.Errorf("số dòng của seller A = %d, mong 2", len(fos[0].LineIDs()))
	}
	if fos[0].Subtotal().Amount() != 500000 {
		t.Errorf("tổng của seller A = %v, mong 500000", fos[0].Subtotal())
	}
}

// Kết quả tách phải ỔN ĐỊNH: chạy lại ra cùng thứ tự và cùng mã FO.
//
// Không ổn định thì mã FO đổi giữa các lần chạy, và seller nhận được mã
// khác với mã đã thông báo cho khách.
func TestKetQuaTachDonOnDinh(t *testing.T) {
	sellers := []ids.ID{
		ids.MustNew(ids.PrefixSeller), ids.MustNew(ids.PrefixSeller),
		ids.MustNew(ids.PrefixSeller), ids.MustNew(ids.PrefixSeller),
	}
	var lines []*domain.Line
	for _, s := range sellers {
		lines = append(lines, newLine(t, s, 100000, 1, 1000))
	}
	o := newOrder(t, lines...)

	first, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	for i := 0; i < 50; i++ {
		got, err := domain.SplitIntoFulfillmentOrders(o, testNow)
		if err != nil {
			t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
		}
		for j := range got {
			if got[j].SellerID() != first[j].SellerID() {
				t.Fatalf("thứ tự đổi giữa các lần gọi ở vị trí %d", j)
			}
			if got[j].FONumber() != first[j].FONumber() {
				t.Fatalf("mã FO đổi: %q rồi %q", first[j].FONumber(), got[j].FONumber())
			}
		}
	}
}

// RANH GIỚI BẢO MẬT nằm trong CẤU TRÚC DỮ LIỆU (ADR-0007, lý do 3).
//
// Seller được xem phần của mình; seller KHÔNG được xem Order (chứa hàng
// của seller khác).
//
// Nếu seller truy cập Order thì phải lọc ở tầng hiển thị, và QUÊN MỘT LẦN
// là rò rỉ dữ liệu đối thủ.
func TestSellerChiThayPhanCuaMinh(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	o := newOrder(t,
		newLine(t, sellerA, 500000, 1, 1000),
		newLine(t, sellerB, 300000, 1, 1000),
	)

	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	foA, foB := fos[0], fos[1]

	// Mỗi FO chỉ thuộc về ĐÚNG một seller.
	if !foA.BelongsTo(sellerA) {
		t.Error("FO của seller A phải thuộc về seller A")
	}
	if foA.BelongsTo(sellerB) {
		t.Error("FO của seller A KHÔNG được thuộc về seller B — rò rỉ dữ liệu")
	}

	// Số tiền của mỗi FO chỉ là phần của seller đó, không lộ tổng đơn.
	if foA.Subtotal().Amount() != 500000 {
		t.Errorf("FO A thấy %v, mong 500000 — không được thấy tổng đơn", foA.Subtotal())
	}
	if foB.Subtotal().Amount() != 300000 {
		t.Errorf("FO B thấy %v, mong 300000", foB.Subtotal())
	}

	// FO không chứa dòng hàng của seller khác.
	linesOfA := foA.LineIDs()
	for _, id := range foB.LineIDs() {
		for _, aID := range linesOfA {
			if id == aID {
				t.Error("dòng hàng xuất hiện ở cả hai FO — ranh giới bị phá")
			}
		}
	}
}

// TRẠNG THÁI TỔNG HỢP SUY RA TỪ FO (quy tắc 7), không tự đặt.
func TestTrangThaiTongHopSuyRaTuFO(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	o := newOrder(t,
		newLine(t, sellerA, 500000, 1, 1000),
		newLine(t, sellerB, 300000, 1, 1000),
	)
	if err := o.MarkPaid(testNow); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := o.MarkProcessing(testNow); err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}

	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}
	foA, foB := fos[0], fos[1]

	advance := func(fo *domain.FulfillmentOrder, to domain.FOStatus) {
		t.Helper()
		steps := []struct {
			st domain.FOStatus
			fn func(time.Time) error
		}{
			{domain.FOConfirmed, fo.Confirm},
			{domain.FOPacked, fo.Pack},
			{domain.FOShipped, fo.Ship},
			{domain.FODelivered, fo.Deliver},
		}
		for _, s := range steps {
			if err := s.fn(testNow); err != nil {
				t.Fatalf("chuyển sang %s: %v", s.st, err)
			}
			if s.st == to {
				return
			}
		}
	}

	// MỘT FO đã xuất → PARTIALLY_SHIPPED.
	advance(foA, domain.FOShipped)
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusPartiallyShipped {
		t.Errorf("một FO đã xuất: trạng thái = %q, mong PARTIALLY_SHIPPED", o.Status())
	}

	// CẢ HAI đã xuất → SHIPPED.
	advance(foB, domain.FOShipped)
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusShipped {
		t.Errorf("cả hai FO đã xuất: trạng thái = %q, mong SHIPPED", o.Status())
	}

	// MỘT FO đã giao → PARTIALLY_DELIVERED.
	if err := foA.Deliver(testNow); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusPartiallyDelivered {
		t.Errorf("một FO đã giao: trạng thái = %q, mong PARTIALLY_DELIVERED", o.Status())
	}

	// CẢ HAI đã giao → DELIVERED.
	if err := foB.Deliver(testNow); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusDelivered {
		t.Errorf("cả hai FO đã giao: trạng thái = %q, mong DELIVERED", o.Status())
	}
}

// Tất cả FO bị hủy → đơn CANCELLED. Một số bị hủy → PARTIALLY_CANCELLED.
func TestTrangThaiKhiFOBiHuy(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	o := newOrder(t,
		newLine(t, sellerA, 500000, 1, 1000),
		newLine(t, sellerB, 300000, 1, 1000),
	)
	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	// Seller B hết hàng, hủy phần của mình.
	if err := fos[1].Cancel("Hết hàng", testNow); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusPartiallyCancelled {
		t.Errorf("trạng thái = %q, mong PARTIALLY_CANCELLED", o.Status())
	}

	// Seller A cũng hủy → cả đơn hủy.
	if err := fos[0].Cancel("Hết hàng", testNow); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusCancelled {
		t.Errorf("trạng thái = %q, mong CANCELLED", o.Status())
	}
}

// Hủy FO BẮT BUỘC nêu lý do: seller và khách đều cần biết vì sao.
func TestHuyFOBatBuocCoLyDo(t *testing.T) {
	o := newOrder(t, newLine(t, ids.MustNew(ids.PrefixSeller), 500000, 1, 1000))
	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	if err := fos[0].Cancel("", testNow); err == nil {
		t.Error("hủy không nêu lý do phải bị chặn")
	}
	if fos[0].Status() == domain.FOCancelled {
		t.Error("hủy thất bại nhưng trạng thái vẫn đổi")
	}
}

// QUY TẮC 8: đã ĐÓNG GÓI thì hủy cần quy trình riêng — có chi phí phát sinh.
func TestDaDongGoiThiKhongHuyThongThuong(t *testing.T) {
	o := newOrder(t, newLine(t, ids.MustNew(ids.PrefixSeller), 500000, 1, 1000))
	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}
	fo := fos[0]

	if err := fo.Confirm(testNow); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	// Chưa đóng gói thì hủy không tốn chi phí.
	if !fo.Status().IsCancellableWithoutCost() {
		t.Error("trạng thái CONFIRMED phải hủy được không tốn chi phí")
	}

	if err := fo.Pack(testNow); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	// Đã đóng gói: hủy thông thường bị chặn.
	if fo.Status().IsCancellableWithoutCost() {
		t.Error("PACKED không được coi là hủy tự do")
	}
	if err := fo.Cancel("Khách đổi ý", testNow); !errors.Is(err, domain.ErrInvalidStatus) {
		t.Errorf("lỗi = %v, mong ErrInvalidStatus — đã đóng gói cần quy trình riêng", err)
	}
}

// DELIVERED và COMPLETED là HAI trạng thái khác nhau (mục 5.3).
//
// Ý NGHĨA TÀI CHÍNH:
//
//	DELIVERED  → số dư seller vẫn PENDING
//	COMPLETED  → số dư chuyển sang AVAILABLE, được payout
//
// Trả tiền seller ngay khi giao hàng thì khi khách hoàn hàng phải đòi lại
// tiền — rất khó thu hồi.
func TestPhanBietDeliveredVaCompleted(t *testing.T) {
	o := newOrder(t, newLine(t, ids.MustNew(ids.PrefixSeller), 500000, 1, 1000))
	fos, err := domain.SplitIntoFulfillmentOrders(o, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}
	fo := fos[0]

	for _, fn := range []func(time.Time) error{fo.Confirm, fo.Pack, fo.Ship, fo.Deliver} {
		if err := fn(testNow); err != nil {
			t.Fatalf("chuyển trạng thái FO: %v", err)
		}
	}
	o.RecalculateStatus(fos, testNow)

	// Đã giao nhưng CHƯA hoàn tất: còn trong hạn đổi trả.
	if o.Status() != domain.StatusDelivered {
		t.Fatalf("trạng thái = %q, mong DELIVERED", o.Status())
	}
	if !o.CompletedAt().IsZero() {
		t.Error("chưa hoàn tất thì completedAt phải rỗng")
	}

	// Hết hạn đổi trả → COMPLETED.
	completedAt := testNow.AddDate(0, 0, 7)
	if err := o.Complete(completedAt); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if o.Status() != domain.StatusCompleted {
		t.Errorf("trạng thái = %q, mong COMPLETED", o.Status())
	}
	if !o.CompletedAt().Equal(completedAt) {
		t.Errorf("completedAt = %v, mong %v", o.CompletedAt(), completedAt)
	}

	// COMPLETED là trạng thái CUỐI: tính lại không làm đổi.
	o.RecalculateStatus(fos, testNow)
	if o.Status() != domain.StatusCompleted {
		t.Error("COMPLETED bị tính lại thành trạng thái khác")
	}
}
