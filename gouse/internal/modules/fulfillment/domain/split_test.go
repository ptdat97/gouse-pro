package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

var testNow = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func line(sellerID ids.ID, total, commission int64) domain.SplitLine {
	return domain.SplitLine{
		LineID:           ids.MustNew(ids.PrefixOrderLine),
		SellerID:         sellerID,
		SKUID:            ids.MustNew(ids.PrefixSKU),
		Quantity:         1,
		LineTotal:        vnd(total),
		CommissionAmount: vnd(commission),
	}
}

func splitInput(lines ...domain.SplitLine) domain.SplitInput {
	return domain.SplitInput{
		OrderID:     ids.MustNew(ids.PrefixOrder),
		OrderNumber: "FC-2026-08-001234",
		Currency:    money.VND,
		Lines:       lines,
	}
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

	in := splitInput(
		line(ownBrand, 299000, 0),
		line(sellerA, 890000, 89000),
		line(sellerB, 900000, 108000),
	)

	fos, err := domain.SplitIntoFulfillmentOrders(in, testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	if len(fos) != 3 {
		t.Fatalf("số đơn thực hiện = %d, mong 3", len(fos))
	}

	// Mã đánh theo thứ tự: -A, -B, -C. Seller thấy mã của mình mà KHÔNG
	// cần biết có bao nhiêu seller khác trong đơn.
	want := []string{"-A", "-B", "-C"}
	for i, fo := range fos {
		suffix := fo.FONumber()[len(fo.FONumber())-2:]
		if suffix != want[i] {
			t.Errorf("mã đơn %d = %q, hậu tố mong %q", i+1, fo.FONumber(), want[i])
		}
		if fo.Status() != domain.FOPending {
			t.Errorf("trạng thái ban đầu = %q, mong PENDING", fo.Status())
		}
	}

	// Số tiền của RIÊNG từng nguồn, để seller đối soát mà không thấy đơn.
	if fos[2].Subtotal().Amount() != 900000 {
		t.Errorf("tiền hàng seller B = %v, mong 900000", fos[2].Subtotal())
	}
	if fos[2].SellerPayable().Amount() != 792000 {
		t.Errorf("phải trả seller B = %v, mong 792000", fos[2].SellerPayable())
	}
}

// NHIỀU DÒNG CÙNG SELLER gom vào MỘT đơn thực hiện.
//
// Hai món của cùng một seller đóng chung một gói được — tách ra là tăng
// chi phí vận chuyển vô ích.
func TestNhieuDongCungSellerGopMotDon(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)

	fos, err := domain.SplitIntoFulfillmentOrders(splitInput(
		line(sellerA, 300000, 30000),
		line(sellerA, 200000, 20000),
	), testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	if len(fos) != 1 {
		t.Fatalf("số đơn thực hiện = %d, mong 1", len(fos))
	}
	if len(fos[0].LineIDs()) != 2 {
		t.Errorf("số dòng hàng = %d, mong 2", len(fos[0].LineIDs()))
	}
	if fos[0].Subtotal().Amount() != 500000 {
		t.Errorf("tiền hàng = %v, mong 500000", fos[0].Subtotal())
	}
}

// KẾT QUẢ TÁCH PHẢI ỔN ĐỊNH: chạy lại ra cùng thứ tự.
//
// Nếu phụ thuộc thứ tự duyệt map, mã FO sẽ đổi giữa các lần chạy — và mọi
// thông báo đã gửi cho seller sẽ trỏ sai chỗ.
func TestKetQuaTachDonOnDinh(t *testing.T) {
	a := ids.MustNew(ids.PrefixSeller)
	b := ids.MustNew(ids.PrefixSeller)
	c := ids.MustNew(ids.PrefixSeller)

	in := splitInput(line(a, 100000, 0), line(b, 200000, 0), line(c, 300000, 0))

	first, err := domain.SplitIntoFulfillmentOrders(in, testNow)
	if err != nil {
		t.Fatalf("lần tách đầu: %v", err)
	}

	// Chạy 50 lần: thứ tự phải giống hệt.
	for i := 0; i < 50; i++ {
		again, err := domain.SplitIntoFulfillmentOrders(in, testNow)
		if err != nil {
			t.Fatalf("lần tách %d: %v", i, err)
		}
		for j := range first {
			if again[j].SellerID() != first[j].SellerID() {
				t.Fatalf("lần %d: vị trí %d là seller %s, lần đầu là %s — "+
					"thứ tự tách không ổn định thì mã FO đổi giữa các lần chạy",
					i, j, again[j].SellerID(), first[j].SellerID())
			}
		}
	}
}

// SELLER CHỈ THẤY PHẦN CỦA MÌNH — hàng rào cuối ở tầng domain.
func TestSellerChiThayPhanCuaMinh(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	fos, err := domain.SplitIntoFulfillmentOrders(splitInput(
		line(sellerA, 300000, 0),
		line(sellerB, 200000, 0),
	), testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}

	foA, foB := fos[0], fos[1]

	if !foA.BelongsTo(sellerA) {
		t.Error("đơn của seller A phải thuộc về seller A")
	}
	if foA.BelongsTo(sellerB) {
		t.Error("đơn của seller A KHÔNG được thuộc về seller B — đây là " +
			"hàng rào cuối cùng ở tầng domain")
	}
	if foB.BelongsTo(sellerA) {
		t.Error("đơn của seller B KHÔNG được thuộc về seller A")
	}
}

// VÒNG ĐỜI ĐẦY ĐỦ: từ chờ xử lý tới hoàn tất.
func TestVongDoiDayDu(t *testing.T) {
	fos, _ := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300000, 0)), testNow)
	fo := fos[0]

	steps := []struct {
		ten  string
		do   func() error
		mong domain.FOStatus
	}{
		{"phân bổ kho", func() error {
			return fo.Allocate(ids.MustNew(ids.PrefixStockLocation), testNow)
		}, domain.FOAllocated},
		{"xác nhận", func() error { return fo.Confirm(testNow) }, domain.FOConfirmed},
		{"lấy hàng", func() error { return fo.Pick(testNow) }, domain.FOPicking},
		{"đóng gói", func() error { return fo.Pack(testNow) }, domain.FOPacked},
		{"bàn giao", func() error {
			return fo.HandOver("GHN", "GHN123456", testNow)
		}, domain.FOHandedOver},
		{"đang giao", func() error { return fo.MarkInTransit(testNow) }, domain.FOInTransit},
		{"đã giao", func() error { return fo.Deliver(testNow) }, domain.FODelivered},
		{"hoàn tất", func() error { return fo.Complete(testNow) }, domain.FOCompleted},
	}

	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.ten, err)
		}
		if fo.Status() != s.mong {
			t.Fatalf("sau %s: trạng thái = %q, mong %q", s.ten, fo.Status(), s.mong)
		}
	}

	if fo.TrackingNumber() != "GHN123456" {
		t.Errorf("mã vận đơn = %q, mong GHN123456", fo.TrackingNumber())
	}
}

// BÀN GIAO VẬN CHUYỂN BẮT BUỘC CÓ MÃ VẬN ĐƠN.
//
// Từ đây hàng ra khỏi tầm kiểm soát của seller. Không có mã thì không ai
// trả lời được "hàng của tôi đang ở đâu".
func TestBanGiaoBatBuocCoMaVanDon(t *testing.T) {
	fos, _ := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300000, 0)), testNow)
	fo := fos[0]

	_ = fo.Confirm(testNow)
	_ = fo.Pack(testNow)

	if err := fo.HandOver("GHN", "", testNow); err == nil {
		t.Error("bàn giao không có mã vận đơn phải bị chặn")
	}
	if fo.Status() != domain.FOPacked {
		t.Errorf("trạng thái = %q, mong giữ nguyên PACKED", fo.Status())
	}
}

// GIAO THẤT BẠI thì giao lại được, hoặc trả về người gửi.
func TestGiaoThatBaiThiGiaoLaiDuoc(t *testing.T) {
	fos, _ := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300000, 0)), testNow)
	fo := fos[0]

	_ = fo.Confirm(testNow)
	_ = fo.Pack(testNow)
	_ = fo.HandOver("GHN", "GHN1", testNow)
	_ = fo.MarkInTransit(testNow)

	// Lý do BẮT BUỘC: khách cần biết vì sao chưa nhận được hàng.
	if err := fo.MarkDeliveryFailed("", testNow); err == nil {
		t.Error("giao thất bại không nêu lý do phải bị chặn")
	}

	if err := fo.MarkDeliveryFailed("khách không có nhà", testNow); err != nil {
		t.Fatalf("MarkDeliveryFailed: %v", err)
	}
	if fo.FailureReason() != "khách không có nhà" {
		t.Errorf("lý do = %q, mong 'khách không có nhà'", fo.FailureReason())
	}

	// Giao lại được.
	if err := fo.MarkInTransit(testNow); err != nil {
		t.Errorf("giao lại phải được: %v", err)
	}
}

// DELIVERED KHÁC COMPLETED — ranh giới TÀI CHÍNH.
//
//	DELIVERED  → số dư seller vẫn Pending
//	COMPLETED  → số dư chuyển Available, seller được chi trả
//
// Đây là cơ chế bảo vệ nền tảng khỏi rủi ro hoàn hàng sau khi đã trả tiền.
func TestPhanBietDeliveredVaCompleted(t *testing.T) {
	fos, _ := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300000, 0)), testNow)
	fo := fos[0]

	_ = fo.Confirm(testNow)
	_ = fo.Pack(testNow)
	_ = fo.HandOver("GHN", "GHN1", testNow)
	_ = fo.Deliver(testNow)

	if fo.Status().IsFinal() {
		t.Error("DELIVERED KHÔNG phải trạng thái cuối — tiền seller vẫn " +
			"đang bị giữ chờ hết hạn đổi trả")
	}

	if err := fo.Complete(testNow); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !fo.Status().IsFinal() {
		t.Error("COMPLETED phải là trạng thái cuối")
	}
	if fo.CompletedAt().IsZero() {
		t.Error("COMPLETED phải có mốc thời gian — đó là mốc tính hạn chi trả")
	}
}

// TỪ PICKING TRỞ ĐI thì hủy cần quy trình riêng (quy tắc 8).
func TestDaLayHangThiKhongHuyThongThuong(t *testing.T) {
	fos, _ := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300000, 0)), testNow)
	fo := fos[0]

	// Trước khi lấy hàng: hủy được.
	if !fo.Status().IsCancellableWithoutCost() {
		t.Error("PENDING phải hủy được không mất phí")
	}

	_ = fo.Confirm(testNow)
	if !fo.Status().IsCancellableWithoutCost() {
		t.Error("CONFIRMED phải hủy được không mất phí")
	}

	_ = fo.Pick(testNow)
	if fo.Status().IsCancellableWithoutCost() {
		t.Error("PICKING đã tốn công lấy hàng — hủy phải qua quy trình riêng")
	}

	if err := fo.Cancel("đổi ý", testNow); !errors.Is(err, domain.ErrInvalidStatus) {
		t.Errorf("lỗi = %v, mong ErrInvalidStatus", err)
	}
}

// HỦY BẮT BUỘC CÓ LÝ DO: seller cần biết vì sao, khách cần lời giải thích.
func TestHuyBatBuocCoLyDo(t *testing.T) {
	fos, _ := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300000, 0)), testNow)
	fo := fos[0]

	if err := fo.Cancel("", testNow); err == nil {
		t.Error("hủy không nêu lý do phải bị chặn")
	}
	if err := fo.Cancel("hết hàng tại kho", testNow); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if fo.CancelReason() != "hết hàng tại kho" {
		t.Errorf("lý do hủy = %q", fo.CancelReason())
	}
}

// TÁCH ĐƠN KHÔNG CÓ DÒNG HÀNG bị chặn.
func TestTachDonRongBiChan(t *testing.T) {
	_, err := domain.SplitIntoFulfillmentOrders(splitInput(), testNow)
	if !errors.Is(err, domain.ErrNoLines) {
		t.Errorf("lỗi = %v, mong ErrNoLines", err)
	}
}

// IsShipped ĐÚNG cho mọi trạng thái sau khi hàng rời kho.
//
// Dùng để tính trạng thái tổng hợp của đơn: đã giao thì cũng đã xuất.
func TestIsShippedDungChoMoiTrangThaiSauKhiRoiKho(t *testing.T) {
	for _, tc := range []struct {
		status domain.FOStatus
		mong   bool
	}{
		{domain.FOPending, false},
		{domain.FOAllocated, false},
		{domain.FOConfirmed, false},
		{domain.FOPicking, false},
		{domain.FOPacked, false},
		{domain.FOHandedOver, true},
		{domain.FOInTransit, true},
		{domain.FODeliveryFailed, true},
		{domain.FODelivered, true},
		{domain.FOCompleted, true},
		{domain.FOCancelled, false},
	} {
		if got := tc.status.IsShipped(); got != tc.mong {
			t.Errorf("%s.IsShipped() = %v, mong %v", tc.status, got, tc.mong)
		}
	}
}
