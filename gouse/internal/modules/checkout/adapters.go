package checkout

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/cart"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// Các adapter dưới đây nối cổng ra của tầng application với API công khai
// của module khác.
//
// Chúng nằm ở tầng này, KHÔNG phải trong application: tầng application chỉ
// biết các interface do chính nó định nghĩa, nên nó kiểm chứng được bằng
// bản giả mà không cần dựng bốn module thật. Đây cũng là điều quy tắc R1
// của archcheck cưỡng chế — chỉ file này được import module khác.

// cartAdapter nối tới module cart.
type cartAdapter struct{ api cart.API }

var _ application.CartPort = (*cartAdapter)(nil)

// LoadPurchasable đọc giỏ và lấy các món MUA ĐƯỢC.
//
// LỌC theo Availability là điểm quan trọng: giỏ giữ cả món hết hàng để
// khách thấy và tự quyết định, nhưng chúng KHÔNG được vào phiên thanh
// toán. Đưa vào rồi mới phát hiện hết hàng nghĩa là khách chờ hết 15 phút
// để nhận thông báo thất bại.
func (a *cartAdapter) LoadPurchasable(
	ctx context.Context, cartID ids.ID,
) (application.CartSnapshot, error) {
	var out application.CartSnapshot

	view, err := a.api.GetCart(ctx, cartID.String())
	if err != nil {
		return out, err
	}

	out = application.CartSnapshot{
		CartID:     ids.ID(view.ID),
		CustomerID: ids.ID(view.CustomerID),
		Currency:   money.Currency(view.Currency),
	}

	for _, it := range view.Items {
		if it.Availability != cart.AvailabilityAvailable &&
			it.Availability != cart.AvailabilityQuantityReduced {
			continue
		}

		price, err := money.New(it.UnitPrice.Value, money.Currency(it.UnitPrice.Currency))
		if err != nil {
			// Giá hỏng thì BỎ QUA món đó, không làm hỏng cả phiên: khách
			// mua được chín món còn lại vẫn hơn là không mua được gì.
			continue
		}

		out.Items = append(out.Items, application.CartItemSnapshot{
			CartItemID:         ids.ID(it.ID),
			OfferID:            ids.ID(it.OfferID),
			SKUID:              ids.ID(it.SKUID),
			SellerID:           ids.ID(it.SellerID),
			ProductName:        it.ProductName,
			VariantDescription: it.VariantDescription,
			UnitPrice:          price,
			Quantity:           it.Quantity,
			SourceContentID:    ids.ID(it.SourceContentID),
			SourceCreatorID:    ids.ID(it.SourceCreatorID),
		})
	}
	return out, nil
}

func (a *cartAdapter) MarkConverted(ctx context.Context, cartID ids.ID) error {
	return a.api.MarkConverted(ctx, cartID.String())
}

// ActiveCartID tra giỏ đang dùng của người mua.
//
// Dùng GetOrCreateCart chứ không phải một hàm "tìm" riêng: khách bấm thanh
// toán khi chưa từng có giỏ sẽ nhận giỏ rỗng, và StartCheckout trả về
// ErrEmptyCart — thông điệp đúng với thứ khách gặp phải. Trả "không tìm
// thấy giỏ" ở đây chỉ khiến giao diện phải đoán xem lỗi nghĩa là gì.
func (a *cartAdapter) ActiveCartID(
	ctx context.Context, customerID, sessionID string,
) (ids.ID, error) {
	view, err := a.api.GetOrCreateCart(ctx, cart.GetOrCreateRequest{
		CustomerID: customerID,
		SessionID:  sessionID,
		Currency:   string(money.VND),
	})
	if err != nil {
		return "", err
	}
	return ids.ID(view.ID), nil
}

// inventoryAdapter nối tới module inventory.
type inventoryAdapter struct{ api inventory.API }

var _ application.InventoryPort = (*inventoryAdapter)(nil)

func (a *inventoryAdapter) FindItemsForSKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID][]application.StockItem, error) {
	strs := make([]string, 0, len(skuIDs))
	for _, id := range skuIDs {
		strs = append(strs, id.String())
	}

	// locationID rỗng = mọi kho. Việc chọn kho nào là quyết định của
	// checkout, không phải của inventory.
	found, err := a.api.GetItemsBySKUs(ctx, strs, "")
	if err != nil {
		return nil, err
	}

	out := make(map[ids.ID][]application.StockItem, len(found))
	for skuID, items := range found {
		list := make([]application.StockItem, 0, len(items))
		for _, it := range items {
			list = append(list, application.StockItem{
				ItemID:    ids.ID(it.ID),
				Available: it.Available,
				OwnerID:   ids.ID(it.OwnerID),
			})
		}
		out[ids.ID(skuID)] = list
	}
	return out, nil
}

func (a *inventoryAdapter) Reserve(
	ctx context.Context, itemID, checkoutID ids.ID, qty int, ttl time.Duration,
) (ids.ID, error) {
	res, err := a.api.Reserve(ctx, inventory.ReserveRequest{
		ItemID:     itemID.String(),
		CheckoutID: checkoutID.String(),
		Quantity:   qty,
		TTL:        ttl,
	})
	if err != nil {
		return "", err
	}
	return ids.ID(res.ID), nil
}

func (a *inventoryAdapter) Release(ctx context.Context, reservationID ids.ID) error {
	return a.api.ReleaseReservation(ctx, reservationID.String())
}

func (a *inventoryAdapter) Extend(
	ctx context.Context, reservationID ids.ID, d time.Duration,
) error {
	return a.api.ExtendReservation(ctx, reservationID.String(), d)
}

// sellerAdapter nối tới module seller.
//
// Nó ghép hai mảnh mà không module nào tự có đủ: `IsInternal` là câu trả
// lời của module seller, còn định danh chủ sở hữu nền tảng là hằng số của
// module inventory. Quy tắc ghép nằm ở inventory.OwnerForSeller — một chỗ
// duy nhất, để hai module không tự định nghĩa lại chuỗi "own_platform"
// rồi lệch nhau.
type sellerAdapter struct{ api seller.API }

var _ application.SellerPort = (*sellerAdapter)(nil)

func (a *sellerAdapter) InventoryOwnerID(
	ctx context.Context, sellerID ids.ID,
) (ids.ID, error) {
	v, err := a.api.GetSeller(ctx, sellerID.String())
	if err != nil {
		return "", err
	}
	return ids.ID(inventory.OwnerForSeller(v.ID, v.IsInternal)), nil
}

// commissionAdapter nối tới module marketplace.
type commissionAdapter struct{ api marketplace.API }

var _ application.CommissionPort = (*commissionAdapter)(nil)

// RateForSeller lấy TỶ LỆ hoa hồng, không phải số tiền.
//
// Tiền được tính MỘT LẦN ở module order khi tạo dòng hàng. Tính ở hai nơi
// sẽ ra hai con số khi quy tắc làm tròn khác nhau, và khi đó không biết
// bên nào đúng.
func (a *commissionAdapter) RateForSeller(
	ctx context.Context, sellerID ids.ID,
) (types.BasisPoints, error) {
	raw, err := a.api.GetCommissionRate(ctx, sellerID.String())
	if err != nil {
		return types.BasisPoints{}, err
	}
	return types.NewBasisPoints(raw)
}

// orderAdapter nối tới module order.
type orderAdapter struct{ api order.API }

var _ application.OrderPort = (*orderAdapter)(nil)

// PlaceOrder truyền THẲNG các con số đã đóng băng sang module order.
//
// Không tính lại gì ở đây. Toàn bộ ý nghĩa của việc đóng băng giá ở bước
// StartCheckout nằm ở chỗ này: con số khách nhìn thấy ở màn hình thanh
// toán là con số đi vào đơn hàng.
func (a *orderAdapter) PlaceOrder(
	ctx context.Context, in application.PlaceOrderInput,
) (application.PlacedOrder, error) {
	lines := make([]order.PlaceOrderLineInput, 0, len(in.Lines))
	for _, l := range in.Lines {
		lines = append(lines, order.PlaceOrderLineInput{
			OfferID:            l.OfferID.String(),
			SKUID:              l.SKUID.String(),
			SellerID:           l.SellerID.String(),
			ProductName:        l.ProductName,
			VariantDescription: l.VariantDescription,
			UnitPrice: order.Amount{
				Value:    l.UnitPrice.Amount(),
				Currency: string(l.UnitPrice.Currency()),
			},
			Quantity:       l.Quantity,
			CommissionRate: int(l.CommissionRate.Value()),
		})
	}

	res, err := a.api.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: in.CustomerID.String(),
		GuestEmail: in.GuestEmail,
		GuestPhone: in.GuestPhone,
		ShippingAddress: order.AddressInput{
			RecipientName: in.ShippingAddress.RecipientName,
			Phone:         in.ShippingAddress.Phone,
			StreetAddress: in.ShippingAddress.StreetAddress,
			Ward:          in.ShippingAddress.Ward,
			District:      in.ShippingAddress.District,
			Province:      in.ShippingAddress.Province,
			CountryCode:   in.ShippingAddress.CountryCode,
		},
		SourceCheckoutID: in.SourceCheckoutID.String(),
		Currency:         string(in.Currency),
		ShippingFee:      toOrderAmount(in.ShippingFee),
		DiscountAmount:   toOrderAmount(in.DiscountAmount),
		TaxAmount:        toOrderAmount(in.TaxAmount),
		Lines:            lines,
		IdempotencyKey:   in.IdempotencyKey,
	})
	if err != nil {
		return application.PlacedOrder{}, err
	}

	return application.PlacedOrder{
		OrderID:     ids.ID(res.Order.ID),
		OrderNumber: res.Order.OrderNumber,
		Replayed:    res.Replayed,
	}, nil
}

func toOrderAmount(m money.Money) order.Amount {
	return order.Amount{Value: m.Amount(), Currency: string(m.Currency())}
}
