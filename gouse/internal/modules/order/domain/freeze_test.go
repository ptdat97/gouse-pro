package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
)

// KIỂM CHỨNG NGUYÊN TẮC ĐÓNG BĂNG bằng đúng tình huống ở mục 4 của đặc tả.
//
//	10/08: Khách mua áo 299.000đ, hoa hồng 10%
//	15/08: Seller giảm giá còn 249.000đ
//	20/08: Nền tảng đổi chính sách hoa hồng thành 12%
//	25/08: Chạy đối soát cho kỳ 01–15/08
//
//	Tham chiếu động: 249.000 × 12% = 29.880đ    ← SAI
//	Đóng băng:       299.000 × 10% = 29.900đ    ← ĐÚNG
//
// Sai lệch này phá vỡ niềm tin của seller và không giải thích được khi có
// tranh chấp.
func TestDongBangGiuNguyenDuLieuDoiSoat(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)

	// Ngày 10/08: đặt hàng với giá 299.000đ và hoa hồng 10%.
	line := newLine(t, sellerID, 299000, 1, 1000)
	o := newOrder(t, line)

	// Con số ĐÓNG BĂNG tại thời điểm đặt hàng.
	if line.UnitPrice().Amount() != 299000 {
		t.Fatalf("đơn giá = %v, mong 299000", line.UnitPrice())
	}
	if line.CommissionAmount().Amount() != 29900 {
		t.Fatalf("hoa hồng = %v, mong 29900", line.CommissionAmount())
	}

	// Ngày 15/08 và 20/08: giá offer đổi, chính sách hoa hồng đổi.
	//
	// Mô phỏng bằng cách tạo dòng MỚI với dữ liệu mới — dòng CŨ trong đơn
	// phải không bị ảnh hưởng. Đó chính là điều đóng băng bảo đảm.
	newPolicy := newLine(t, sellerID, 249000, 1, 1200)
	if newPolicy.CommissionAmount().Amount() != 29880 {
		t.Fatalf("hoa hồng theo chính sách mới = %v, mong 29880", newPolicy.CommissionAmount())
	}

	// Ngày 25/08: đối soát kỳ 01–15/08 phải ra con số CŨ.
	got, _ := o.LineByID(line.ID())
	if got.CommissionAmount().Amount() != 29900 {
		t.Errorf("đối soát ra %v, mong 29900 — dữ liệu không được đóng băng",
			got.CommissionAmount())
	}
	if got.UnitPrice().Amount() != 299000 {
		t.Errorf("đơn giá đối soát = %v, mong 299000", got.UnitPrice())
	}
	// Tiền phải trả seller cũng dùng số cũ: 299.000 − 29.900 = 269.100.
	if got.SellerPayable().Amount() != 269100 {
		t.Errorf("phải trả seller = %v, mong 269100", got.SellerPayable())
	}
}

// Tên sản phẩm và mô tả biến thể cũng ĐÓNG BĂNG.
//
// Seller đổi tên sản phẩm → hóa đơn cũ vẫn phải đúng.
// Sửa variant → vẫn biết khách mua size nào.
func TestDongBangTenSanPhamVaBienThe(t *testing.T) {
	l, err := domain.NewLine(domain.NewLineParams{
		OfferID:            ids.MustNew(ids.PrefixOffer),
		SKUID:              ids.MustNew(ids.PrefixSKU),
		SellerID:           ids.MustNew(ids.PrefixSeller),
		ProductName:        "Áo sơ mi linen Oxford",
		VariantDescription: "Trắng / M",
		UnitPrice:          vnd(299000),
		Quantity:           1,
		CommissionRate:     bp(t, 1000),
		Now:                testNow,
	})
	if err != nil {
		t.Fatalf("NewLine: %v", err)
	}

	// Không có phương thức nào sửa được tên hay mô tả sau khi tạo —
	// đó là cách đóng băng được cưỡng chế: KHÔNG CÓ SETTER.
	if l.ProductName() != "Áo sơ mi linen Oxford" {
		t.Errorf("tên sản phẩm = %q", l.ProductName())
	}
	if l.VariantDescription() != "Trắng / M" {
		t.Errorf("mô tả biến thể = %q", l.VariantDescription())
	}
}

// Tên sản phẩm là BẮT BUỘC: thiếu thì hóa đơn hiện một dòng vô danh.
func TestTenSanPhamBatBuocKhiDongBang(t *testing.T) {
	_, err := domain.NewLine(domain.NewLineParams{
		OfferID:        ids.MustNew(ids.PrefixOffer),
		SellerID:       ids.MustNew(ids.PrefixSeller),
		UnitPrice:      vnd(100000),
		Quantity:       1,
		CommissionRate: bp(t, 1000),
		Now:            testNow,
	})
	if err == nil {
		t.Error("thiếu tên sản phẩm phải bị chặn")
	}
}

// QUY TẮC 6: khách vãng lai được đặt hàng — giảm rào cản chuyển đổi.
//
// Nhưng phải có cách liên hệ: đơn không biết thuộc về ai và không liên hệ
// được là đơn không giao được.
func TestKhachVangLaiDatDuocHang(t *testing.T) {
	line := newLine(t, ids.MustNew(ids.PrefixSeller), 299000, 1, 1000)

	// Có email khách vãng lai → được.
	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber:    "FC-2026-08-000001",
		GuestEmail:     "khach@example.com",
		GuestPhone:     "0900000000",
		Lines:          []*domain.Line{line},
		IdempotencyKey: "k1",
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("khách vãng lai phải đặt được hàng: %v", err)
	}
	if !o.IsGuestOrder() {
		t.Error("đơn không có customerID phải là đơn khách vãng lai")
	}

	// Không có cả customerID lẫn email → bị chặn.
	line2 := newLine(t, ids.MustNew(ids.PrefixSeller), 299000, 1, 1000)
	_, err = domain.NewOrder(domain.NewOrderParams{
		OrderNumber:    "FC-2026-08-000002",
		Lines:          []*domain.Line{line2},
		IdempotencyKey: "k2",
		Now:            testNow,
	})
	if !errors.Is(err, domain.ErrNoCustomer) {
		t.Errorf("lỗi = %v, mong ErrNoCustomer", err)
	}
}

// QUY TẮC 5: PlaceOrder phải idempotent — khóa là BẮT BUỘC.
func TestKhoaIdempotencyBatBuoc(t *testing.T) {
	line := newLine(t, ids.MustNew(ids.PrefixSeller), 299000, 1, 1000)
	_, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber: "FC-2026-08-000003",
		CustomerID:  ids.MustNew(ids.PrefixCustomer),
		Lines:       []*domain.Line{line},
		Now:         testNow,
	})
	if !errors.Is(err, domain.ErrMissingIdempKey) {
		t.Errorf("lỗi = %v, mong ErrMissingIdempKey", err)
	}
}

// HỦY TỪNG PHẦN: seller B hết hàng, hai seller còn lại vẫn giao được.
//
// Tổng tiền tự động giảm; phí ship KHÔNG thu lại (mục 6.3).
func TestHuyTungPhanKhongThuLaiPhiShip(t *testing.T) {
	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	lineA := newLine(t, sellerA, 400000, 1, 1000)
	lineB := newLine(t, sellerB, 200000, 1, 1000)

	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber: "FC-2026-08-000004",
		CustomerID:  ids.MustNew(ids.PrefixCustomer),
		// Đơn 600.000đ đạt ngưỡng miễn phí ship.
		ShippingFee:    vnd(0),
		Lines:          []*domain.Line{lineA, lineB},
		IdempotencyKey: "k4",
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}

	if o.Total().Amount() != 600000 {
		t.Fatalf("tổng ban đầu = %v, mong 600000", o.Total())
	}

	// Seller B hết hàng → hủy dòng của B.
	if err := o.CancelLine(lineB.ID(), testNow); err != nil {
		t.Fatalf("CancelLine: %v", err)
	}

	if o.Status() != domain.StatusPartiallyCancelled {
		t.Errorf("trạng thái = %q, mong PARTIALLY_CANCELLED", o.Status())
	}
	// Tổng giảm còn 400.000 — tính từ các dòng CÒN HIỆU LỰC.
	if o.Total().Amount() != 400000 {
		t.Errorf("tổng sau hủy = %v, mong 400000", o.Total())
	}
	// Phí ship KHÔNG bị thu lại dù đơn không còn đạt ngưỡng miễn phí:
	// khách không nên bị phạt vì lỗi của seller.
	if o.ShippingFee().Amount() != 0 {
		t.Errorf("phí ship = %v, mong giữ nguyên 0 — không thu lại khi hủy một phần",
			o.ShippingFee())
	}

	// Dòng của seller A vẫn còn hiệu lực.
	if len(o.ActiveLines()) != 1 {
		t.Errorf("số dòng còn hiệu lực = %d, mong 1", len(o.ActiveLines()))
	}
	// Dòng đã hủy KHÔNG bị xóa: quy tắc 3, không xóa dữ liệu đơn hàng.
	if len(o.Lines()) != 2 {
		t.Errorf("tổng số dòng = %d, mong 2 — dòng đã hủy không được xóa", len(o.Lines()))
	}
}

// Hủy dòng CUỐI CÙNG nghĩa là hủy cả đơn.
func TestHuyDongCuoiCungLaHuyCaDon(t *testing.T) {
	line := newLine(t, ids.MustNew(ids.PrefixSeller), 299000, 1, 1000)
	o := newOrder(t, line)

	if err := o.CancelLine(line.ID(), testNow); err != nil {
		t.Fatalf("CancelLine: %v", err)
	}
	if o.Status() != domain.StatusCancelled {
		t.Errorf("trạng thái = %q, mong CANCELLED", o.Status())
	}
}

// ADJUSTMENT là THỰC THỂ HẠNG NHẤT, giải quyết bài toán hoàn tiền từng phần.
//
//	Đơn 3 món, tổng 500.000đ, giảm 50.000đ (10%)
//
//	Không có Adjustment: order.discount_amount = 50000
//	  → khách trả món C (100.000đ), hoàn bao nhiêu? Phải tính lại, dễ sai.
//
//	Có Adjustment: mỗi dòng có khoản giảm riêng
//	  → khách trả món C, hoàn 100.000 − 10.000 = 90.000đ. ĐỌC TRỰC TIẾP.
func TestAdjustmentGiaiQuyetHoanTienTungPhan(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)

	// Ba món: 200.000 + 200.000 + 100.000 = 500.000.
	a := newLine(t, sellerID, 200000, 1, 1000)
	b := newLine(t, sellerID, 200000, 1, 1000)
	c := newLine(t, sellerID, 100000, 1, 1000)
	_ = newOrder(t, a, b, c) // đơn giữ ba dòng; test thao tác trực tiếp trên dòng

	// Giảm 10% phân bổ theo tỷ lệ NGAY KHI ĐẶT HÀNG.
	for _, tc := range []struct {
		line   *domain.Line
		amount int64
	}{{a, -20000}, {b, -20000}, {c, -10000}} {
		adj, err := domain.NewAdjustment(
			domain.AdjustmentPromotion, "Giảm giá THUDONG10",
			vnd(tc.amount), "PROMOTION", ids.MustNew(ids.PrefixCampaign),
			domain.BearerPlatform, testNow)
		if err != nil {
			t.Fatalf("NewAdjustment: %v", err)
		}
		if err := tc.line.AddAdjustment(adj, testNow); err != nil {
			t.Fatalf("AddAdjustment: %v", err)
		}
	}

	// Khách trả món C: số tiền hoàn ĐỌC TRỰC TIẾP, không tính lại tỷ lệ.
	refund, _ := c.LineTotal().Add(c.AdjustmentTotal())
	if refund.Amount() != 90000 {
		t.Errorf("tiền hoàn món C = %v, mong 90000", refund)
	}

	// Và biết được AI chịu chi phí khoản giảm đó.
	adjs := c.Adjustments()
	if len(adjs) != 1 {
		t.Fatalf("số khoản điều chỉnh = %d, mong 1", len(adjs))
	}
	if adjs[0].CostBearer != domain.BearerPlatform {
		t.Errorf("bên chịu chi phí = %q, mong PLATFORM", adjs[0].CostBearer)
	}
	if !adjs[0].IsDiscount() {
		t.Error("khoản âm phải là khoản giảm")
	}
}

// Khoản điều chỉnh BẮT BUỘC có nhãn: khách nhìn thấy nó trên hóa đơn.
//
// Không có nhãn thì hóa đơn hiện một khoản trừ vô danh — khách không hiểu
// và sẽ khiếu nại.
func TestAdjustmentBatBuocCoNhan(t *testing.T) {
	_, err := domain.NewAdjustment(
		domain.AdjustmentPromotion, "", vnd(-10000),
		"PROMOTION", ids.MustNew(ids.PrefixCampaign), domain.BearerPlatform, testNow)
	if err == nil {
		t.Error("khoản điều chỉnh không có nhãn phải bị chặn")
	}

	// Khoản bằng 0 cũng vô nghĩa.
	_, err = domain.NewAdjustment(
		domain.AdjustmentPromotion, "Giảm giá", vnd(0),
		"PROMOTION", ids.MustNew(ids.PrefixCampaign), domain.BearerPlatform, testNow)
	if err == nil {
		t.Error("khoản điều chỉnh bằng 0 phải bị chặn")
	}
}

// Đơn phải có ít nhất một dòng hàng.
func TestDonPhaiCoItNhatMotDong(t *testing.T) {
	_, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber:    "FC-2026-08-000005",
		CustomerID:     ids.MustNew(ids.PrefixCustomer),
		IdempotencyKey: "k5",
		Now:            testNow,
	})
	if !errors.Is(err, domain.ErrNoLines) {
		t.Errorf("lỗi = %v, mong ErrNoLines", err)
	}
}

// Tổng tiền: subtotal + phí ship + thuế − giảm giá.
func TestTinhTongTienDonHang(t *testing.T) {
	line := newLine(t, ids.MustNew(ids.PrefixSeller), 500000, 2, 1000)

	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber:    "FC-2026-08-000006",
		CustomerID:     ids.MustNew(ids.PrefixCustomer),
		ShippingFee:    vnd(30000),
		TaxAmount:      vnd(50000),
		DiscountAmount: vnd(100000),
		Lines:          []*domain.Line{line},
		IdempotencyKey: "k6",
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}

	// 1.000.000 + 30.000 + 50.000 − 100.000 = 980.000.
	if o.Subtotal().Amount() != 1000000 {
		t.Errorf("subtotal = %v, mong 1000000", o.Subtotal())
	}
	if o.Total().Amount() != 980000 {
		t.Errorf("tổng = %v, mong 980000", o.Total())
	}
}

// Không hủy được đơn đã ở trạng thái cuối.
func TestKhongHuyDuocDonDaKetThuc(t *testing.T) {
	line := newLine(t, ids.MustNew(ids.PrefixSeller), 299000, 1, 1000)
	o := newOrder(t, line)

	if err := o.Cancel(testNow); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	// Hủy lần hai bị chặn.
	if err := o.Cancel(testNow); !errors.Is(err, domain.ErrNotCancellable) {
		t.Errorf("lỗi = %v, mong ErrNotCancellable", err)
	}
	// Mọi dòng cũng đã hủy theo.
	for _, l := range o.Lines() {
		if l.Status() != domain.LineCancelled {
			t.Errorf("dòng %s = %q, mong CANCELLED", l.ID(), l.Status())
		}
	}
}
