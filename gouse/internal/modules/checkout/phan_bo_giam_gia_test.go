package checkout_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"
)

// promoThat phân bổ giảm giá THEO TỶ LỆ, rải phần dư.
//
// Dùng bản riêng thay vì gọi module promotion để bài này đo ĐÚNG một thứ:
// checkout CÓ gọi phân bổ và CÓ đóng băng kết quả vào đơn không. Quy tắc
// chia đã có bài test riêng ở module promotion.
type promoThat struct{ goi int }

func (p *promoThat) ValidateCoupon(
	_ context.Context, _, _ string, _ money.Money,
) (money.Money, bool, error) {
	return money.Money{}, false, nil
}

func (p *promoThat) PhanBoGiamGia(
	_ context.Context, giam money.Money, dong []application.DongPhanBo,
) (map[ids.ID]money.Money, error) {
	p.goi++

	tong := int64(0)
	for _, d := range dong {
		tong += d.Total.Amount()
	}

	out := map[ids.ID]money.Money{}
	daChia := int64(0)
	for i, d := range dong {
		var phan int64
		if i == len(dong)-1 {
			// Dòng cuối nhận phần CÒN LẠI: tổng các phần phải bằng ĐÚNG
			// số tiền giảm, phần dư của phép chia không được biến mất.
			phan = giam.Amount() - daChia
		} else {
			phan = giam.Amount() * d.Total.Amount() / tong
			daChia += phan
		}
		m, err := money.New(phan, giam.Currency())
		if err != nil {
			return nil, err
		}
		out[d.LineID] = m
	}
	return out, nil
}

// TestGiamGiaDuocDongBangXuongTungDongHang.
//
// # Vì sao bất biến này quan trọng
//
// Đơn chỉ có tổng giảm ở CẤP ĐƠN thì lúc khách trả một món, không ai tính
// được họ đã thực trả bao nhiêu cho món đó. Hoàn theo giá niêm yết khi ấy
// là trả nhiều hơn đã thu — docs/07-workflows/return.md mục 5 gọi đây là
// điểm dễ sai nhất của cả luồng trả hàng.
//
// Module returns hiện TỪ CHỐI hoàn tự động những đơn có giảm giá chưa phân
// bổ. Bài này khóa mắt xích khiến việc từ chối đó không còn cần thiết.
func TestGiamGiaDuocDongBangXuongTungDongHang(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Dựng service RIÊNG có cổng khuyến mãi: harness mặc định không có,
	// và thêm setter chỉ để test là làm bẩn mã production.
	promo := &promoThat{}
	svc := application.NewService(application.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(h.db.Pool()),
		Carts:       h.cart,
		Inventory:   &realInventory{api: h.inv},
		Commissions: &fakeCommission{rate: 1000},
		Sellers:     h.owners,
		Orders:      checkout.NewOrderPort(h.ord),
		Promotions:  promo,
		Clock:       h.clock,
	})

	// Ba món, tổng 500.000đ — đúng ví dụ của tài liệu.
	sku1 := h.stockSKU(t, 10)
	sku2 := h.stockSKU(t, 10)
	sku3 := h.stockSKU(t, 10)
	h.setCart(
		item(sku1, 200_000, 1),
		item(sku2, 200_000, 1),
		item(sku3, 100_000, 1),
	)

	c, err := svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	// Giảm 50.000đ ở CẤP ĐƠN.
	if _, err := svc.ApplyDiscount(ctx, c.ID(), "GIAM10",
		money.MustNew(50_000, money.VND)); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}
	if _, err := svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}

	res, err := svc.CompleteCheckout(ctx, c.ID(), "phan-bo-giam-gia-1")
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}

	if promo.goi == 0 {
		t.Fatal("checkout KHÔNG gọi phân bổ giảm giá — đơn tạo ra sẽ không " +
			"trả hàng tự động được")
	}

	// Tổng khoản điều chỉnh trên các dòng phải bằng ĐÚNG số tiền giảm.
	var tongDieuChinh int64
	if err := h.db.Pool().QueryRow(ctx, `
		SELECT coalesce(sum(a.amount), 0)
		  FROM order_line_adjustment a
		  JOIN order_line l ON l.id = a.order_line_id
		 WHERE l.order_id = $1`, res.OrderID.String()).Scan(&tongDieuChinh); err != nil {
		t.Fatalf("đọc khoản điều chỉnh: %v", err)
	}

	if tongDieuChinh != -50_000 {
		t.Errorf("tổng khoản điều chỉnh là %d, cần -50000 — phần giảm không "+
			"được đóng băng đủ xuống dòng hàng", tongDieuChinh)
	}
}
