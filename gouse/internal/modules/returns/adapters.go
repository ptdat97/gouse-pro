package returns

import (
	"context"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/modules/returns/application"
)

// Các adapter nối cổng ra của tầng application với API công khai của module
// khác. returns ở tầng giao dịch; order, payment, inventory đều nằm cùng
// tầng hoặc thấp hơn.

// ---------------------------------------------------------------- order

type orderAdapter struct{ api order.API }

var _ application.OrderPort = (*orderAdapter)(nil)

// LayDonDeTraHang đổi góc nhìn đơn hàng thành đúng thứ việc trả hàng cần.
//
// Chỉ lấy trường dùng tới. Trả nguyên OrderView xuống tầng application sẽ
// buộc nó biết mọi thứ về đơn hàng, và mỗi lần order thêm trường là một
// lần returns phải đọc lại.
func (a *orderAdapter) LayDonDeTraHang(
	ctx context.Context, orderID ids.ID,
) (application.DonHang, error) {
	v, err := a.api.GetOrder(ctx, orderID.String())
	if err != nil {
		return application.DonHang{}, err
	}

	don := application.DonHang{
		ID: ids.ID(v.ID),
		// Trả hàng chỉ mở sau khi hàng ĐÃ tới tay khách. Cho phép sớm hơn
		// nghĩa là hoàn tiền cho món vẫn đang trên đường.
		DaGiao: v.Status == order.StatusDelivered || v.Status == order.StatusCompleted,
	}

	giam, err := toMoney(v.DiscountAmount)
	if err != nil {
		return application.DonHang{}, err
	}
	don.GiamGiaCapDon = giam

	for _, l := range v.Lines {
		tong, err := toMoney(l.LineTotal)
		if err != nil {
			return application.DonHang{}, err
		}
		hoaHong, err := toMoney(l.CommissionAmount)
		if err != nil {
			return application.DonHang{}, err
		}

		dieuChinh := money.Money{}
		for _, adj := range l.Adjustments {
			m, err := toMoney(adj.Amount)
			if err != nil {
				return application.DonHang{}, err
			}
			if dieuChinh.IsZero() {
				dieuChinh = m
				continue
			}
			if dieuChinh, err = dieuChinh.Add(m); err != nil {
				return application.DonHang{}, err
			}
		}
		if dieuChinh.IsZero() {
			if dieuChinh, err = money.New(0, tong.Currency()); err != nil {
				return application.DonHang{}, err
			}
		}

		don.Dong = append(don.Dong, application.DongDonHang{
			ID: ids.ID(l.ID), SKUID: ids.ID(l.SKUID), SellerID: ids.ID(l.SellerID),
			Quantity: l.Quantity, LineTotal: tong,
			TongDieuChinh: dieuChinh, HoaHong: hoaHong,
		})
	}
	return don, nil
}

// ---------------------------------------------------------------- inventory

type inventoryAdapter struct {
	api   inventory.API
	owner OwnerResolver
}

var _ application.InventoryPort = (*inventoryAdapter)(nil)

// OwnerResolver đổi mã nhà bán thành chủ sở hữu tồn kho.
//
// Nhà bán nội bộ giữ hàng dưới danh nghĩa NỀN TẢNG, nên hai thứ không phải
// lúc nào cũng là một — xem inventory.OwnerForSeller.
type OwnerResolver interface {
	InventoryOwnerID(ctx context.Context, sellerID string) (string, error)
}

func (a *inventoryAdapter) NhanHangHoan(
	ctx context.Context, skuID, sellerID ids.ID, qty int, returnID ids.ID,
) error {
	item, err := a.timBanGhi(ctx, skuID, sellerID)
	if err != nil {
		return err
	}

	// Hàng vào trạng thái Returned, KHÔNG phải Available — quy tắc bắt
	// buộc của docs/07-workflows/return.md mục 4. Bước kiểm định mới quyết
	// định món hàng đi đâu tiếp.
	return a.api.ReceiveReturn(ctx, item.ID, qty, returnID.String())
}

// timBanGhi tra bản ghi tồn kho của một SKU thuộc một nhà bán.
//
// Tách ra vì cả nhận hàng lẫn kiểm định đều cần: hai bản sao của phép tra
// này là hai chỗ để quên lọc theo chủ sở hữu.
func (a *inventoryAdapter) timBanGhi(
	ctx context.Context, skuID, sellerID ids.ID,
) (*inventory.ItemView, error) {
	chuSoHuu := sellerID.String()
	if a.owner != nil {
		v, err := a.owner.InventoryOwnerID(ctx, sellerID.String())
		if err != nil {
			return nil, err
		}
		chuSoHuu = v
	}

	// Tham số thứ ba là KHO, không phải chủ sở hữu — để rỗng rồi tự lọc.
	found, err := a.api.GetItemsBySKUs(ctx, []string{skuID.String()}, "")
	if err != nil {
		return nil, err
	}

	// Lọc theo CHỦ SỞ HỮU: một SKU có tồn kho của nhiều chủ cùng lúc, và
	// đó là lý do cái chợ tồn tại. Ghi vào bản ghi của người khác nghĩa là
	// cộng hàng cho nhà bán không liên quan.
	for i := range found[skuID.String()] {
		if found[skuID.String()][i].OwnerID == chuSoHuu {
			return &found[skuID.String()][i], nil
		}
	}
	return nil, fmt.Errorf(
		"returns: không có bản ghi tồn kho cho SKU %s của chủ sở hữu %s",
		skuID, chuSoHuu)
}

// GhiKetQuaKiemDinh chuyển hàng hoàn sang Available hoặc Damaged.
func (a *inventoryAdapter) GhiKetQuaKiemDinh(
	ctx context.Context, skuID, sellerID ids.ID, qty int, dat bool, lyDo string,
) error {
	item, err := a.timBanGhi(ctx, skuID, sellerID)
	if err != nil {
		return err
	}
	return a.api.ProcessReturnInspection(ctx, inventory.InspectionRequest{
		ItemID: item.ID, Quantity: qty, Passed: dat,
		Reason: lyDo, PerformedBy: "returns",
	})
}

// ---------------------------------------------------------------- payment

type paymentAdapter struct{ api payment.API }

var _ application.PaymentPort = (*paymentAdapter)(nil)

func (a *paymentAdapter) GhiHoanTien(
	ctx context.Context, in application.HoanTienInput,
) error {
	tien := func(m money.Money) payment.Amount {
		return payment.Amount{Value: m.Amount(), Currency: string(m.Currency())}
	}
	_, err := a.api.RecordRefund(ctx, payment.RefundRequest{
		OrderID:  in.OrderID.String(),
		RefundID: in.ReturnID.String(),
		Amount:   tien(in.TongHoan),

		SellerID:         in.SellerID.String(),
		SellerClawback:   tien(in.DaoNhaBan),
		PlatformClawback: tien(in.DaoHoaHong),

		CreatedBy: "returns",
	})
	return err
}

func toMoney(a order.Amount) (money.Money, error) {
	return money.New(a.Value, money.Currency(a.Currency))
}
