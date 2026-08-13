package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// FOStatus là trạng thái một đơn vị công việc vận hành.
type FOStatus string

const (
	FOPending   FOStatus = "PENDING"
	FOConfirmed FOStatus = "CONFIRMED"
	FOPacked    FOStatus = "PACKED"
	FOShipped   FOStatus = "SHIPPED"
	FODelivered FOStatus = "DELIVERED"
	FOCancelled FOStatus = "CANCELLED"
)

// canTransitionTo mã hóa vòng đời của một FulfillmentOrder.
func (s FOStatus) canTransitionTo(next FOStatus) bool {
	switch s {
	case FOPending:
		return next == FOConfirmed || next == FOCancelled
	case FOConfirmed:
		return next == FOPacked || next == FOCancelled
	case FOPacked:
		// ĐÃ ĐÓNG GÓI thì hủy cần quy trình riêng (quy tắc 8): có chi phí
		// phát sinh — công đóng gói, vật tư, và có thể đã bàn giao vận chuyển.
		return next == FOShipped
	case FOShipped:
		return next == FODelivered
	case FODelivered, FOCancelled:
		// Trạng thái cuối.
		return false
	}
	return false
}

// IsCancellableWithoutCost cho biết hủy ở trạng thái này có phát sinh chi
// phí không.
//
// Từ PACKED trở đi thì đã tốn công đóng gói và vật tư — hủy cần quy trình
// riêng, không phải thao tác thông thường (quy tắc 8).
func (s FOStatus) IsCancellableWithoutCost() bool {
	return s == FOPending || s == FOConfirmed
}

// FulfillmentOrder là ĐƠN VỊ CÔNG VIỆC VẬN HÀNH của MỘT nguồn hàng.
//
// TÁCH KHỎI Order LÀ QUYẾT ĐỊNH CỐT LÕI (ADR-0007, quyết định 2). Năm lý do,
// trong đó hai lý do là QUYẾT ĐỊNH:
//
//  3. RÀNG BUỘC BẢO MẬT
//     Seller được xem phần của mình.
//     Seller KHÔNG được xem Order (chứa hàng của seller khác).
//
//  4. TRANH CHẤP GHI
//     Ba seller cập nhật đồng thời sẽ tranh chấp trên cùng một bản ghi
//     Order nếu gộp chung.
//
// Điểm quan trọng nhất về bảo mật: nếu seller truy cập Order thì phải lọc
// dữ liệu ở tầng hiển thị, và QUÊN MỘT LẦN là rò rỉ dữ liệu đối thủ. Với
// FulfillmentOrder, ranh giới nằm sẵn trong CẤU TRÚC DỮ LIỆU — truy vấn
// theo sellerID tự nhiên chỉ trả phần của họ.
type FulfillmentOrder struct {
	id ids.ID

	// orderID trỏ về hợp đồng gốc với khách.
	orderID ids.ID

	// foNumber là mã hiển thị cho seller, dạng <order_number>-A, -B, -C.
	foNumber string

	// sellerID là CHỦ SỞ HỮU của đơn vị công việc này.
	//
	// Đây là trường tạo nên ranh giới bảo mật: mọi truy vấn của seller đều
	// lọc theo cột này, ngay trong SQL.
	sellerID ids.ID

	// lineIDs là các dòng hàng thuộc nguồn này.
	lineIDs []ids.ID

	status FOStatus

	// Số tiền của riêng phần này, để seller đối soát.
	subtotal         money.Money
	commissionAmount money.Money

	// cancelReason bắt buộc khi hủy: seller và khách đều cần biết vì sao.
	cancelReason string

	confirmedAt time.Time
	packedAt    time.Time
	shippedAt   time.Time
	deliveredAt time.Time
	cancelledAt time.Time

	createdAt time.Time
	updatedAt time.Time
}

type NewFulfillmentOrderParams struct {
	OrderID          ids.ID
	FONumber         string
	SellerID         ids.ID
	LineIDs          []ids.ID
	Subtotal         money.Money
	CommissionAmount money.Money
	Now              time.Time
}

// NewFulfillmentOrder tạo một đơn vị công việc vận hành.
func NewFulfillmentOrder(p NewFulfillmentOrderParams) (*FulfillmentOrder, error) {
	if p.OrderID.IsZero() {
		return nil, errors.New("order: đơn thực hiện phải trỏ về đơn hàng gốc")
	}
	if p.SellerID.IsZero() {
		return nil, errors.New("order: đơn thực hiện phải có nhà bán")
	}
	if len(p.LineIDs) == 0 {
		return nil, errors.New("order: đơn thực hiện phải có ít nhất một dòng hàng")
	}

	id, err := ids.New(ids.PrefixFulfillmentOrder)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &FulfillmentOrder{
		id:               id,
		orderID:          p.OrderID,
		foNumber:         p.FONumber,
		sellerID:         p.SellerID,
		lineIDs:          append([]ids.ID(nil), p.LineIDs...),
		status:           FOPending,
		subtotal:         p.Subtotal,
		commissionAmount: p.CommissionAmount,
		createdAt:        now,
		updatedAt:        now,
	}, nil
}

// RestoreFOParams dựng lại từ kho lưu trữ.
type RestoreFOParams struct {
	ID               ids.ID
	OrderID          ids.ID
	FONumber         string
	SellerID         ids.ID
	LineIDs          []ids.ID
	Status           FOStatus
	Subtotal         money.Money
	CommissionAmount money.Money
	CancelReason     string
	ConfirmedAt      time.Time
	PackedAt         time.Time
	ShippedAt        time.Time
	DeliveredAt      time.Time
	CancelledAt      time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// RestoreFulfillmentOrder dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreFulfillmentOrder(p RestoreFOParams) *FulfillmentOrder {
	return &FulfillmentOrder{
		id:               p.ID,
		orderID:          p.OrderID,
		foNumber:         p.FONumber,
		sellerID:         p.SellerID,
		lineIDs:          p.LineIDs,
		status:           p.Status,
		subtotal:         p.Subtotal,
		commissionAmount: p.CommissionAmount,
		cancelReason:     p.CancelReason,
		confirmedAt:      p.ConfirmedAt,
		packedAt:         p.PackedAt,
		shippedAt:        p.ShippedAt,
		deliveredAt:      p.DeliveredAt,
		cancelledAt:      p.CancelledAt,
		createdAt:        p.CreatedAt,
		updatedAt:        p.UpdatedAt,
	}
}

func (f *FulfillmentOrder) ID() ids.ID                    { return f.id }
func (f *FulfillmentOrder) OrderID() ids.ID               { return f.orderID }
func (f *FulfillmentOrder) FONumber() string              { return f.foNumber }
func (f *FulfillmentOrder) SellerID() ids.ID              { return f.sellerID }
func (f *FulfillmentOrder) Status() FOStatus              { return f.status }
func (f *FulfillmentOrder) Subtotal() money.Money         { return f.subtotal }
func (f *FulfillmentOrder) CommissionAmount() money.Money { return f.commissionAmount }
func (f *FulfillmentOrder) CancelReason() string          { return f.cancelReason }
func (f *FulfillmentOrder) ConfirmedAt() time.Time        { return f.confirmedAt }
func (f *FulfillmentOrder) PackedAt() time.Time           { return f.packedAt }
func (f *FulfillmentOrder) ShippedAt() time.Time          { return f.shippedAt }
func (f *FulfillmentOrder) DeliveredAt() time.Time        { return f.deliveredAt }
func (f *FulfillmentOrder) CancelledAt() time.Time        { return f.cancelledAt }
func (f *FulfillmentOrder) CreatedAt() time.Time          { return f.createdAt }
func (f *FulfillmentOrder) UpdatedAt() time.Time          { return f.updatedAt }

// LineIDs trả bản sao danh sách dòng hàng.
func (f *FulfillmentOrder) LineIDs() []ids.ID {
	return append([]ids.ID(nil), f.lineIDs...)
}

// SellerPayable là số tiền phải trả seller cho phần này.
func (f *FulfillmentOrder) SellerPayable() money.Money {
	payable, _ := f.subtotal.Sub(f.commissionAmount)
	return payable
}

// BelongsTo cho biết đơn thực hiện này có thuộc về seller không.
//
// Hàng rào cuối cùng ở tầng domain. Truy vấn đã lọc theo sellerID, nhưng
// một lần gọi FindByID với id lấy từ nơi khác vẫn có thể lọt — hàm này để
// tầng application kiểm tra tường minh.
func (f *FulfillmentOrder) BelongsTo(sellerID ids.ID) bool {
	return f.sellerID == sellerID
}

// ---------------------------------------------------------------- Hành vi

func (f *FulfillmentOrder) Confirm(now time.Time) error {
	if err := f.transition(FOConfirmed, now); err != nil {
		return err
	}
	f.confirmedAt = now
	return nil
}

func (f *FulfillmentOrder) Pack(now time.Time) error {
	if err := f.transition(FOPacked, now); err != nil {
		return err
	}
	f.packedAt = now
	return nil
}

func (f *FulfillmentOrder) Ship(now time.Time) error {
	if err := f.transition(FOShipped, now); err != nil {
		return err
	}
	f.shippedAt = now
	return nil
}

func (f *FulfillmentOrder) Deliver(now time.Time) error {
	if err := f.transition(FODelivered, now); err != nil {
		return err
	}
	f.deliveredAt = now
	return nil
}

// Cancel hủy đơn vị công việc này.
//
// Lý do là BẮT BUỘC: seller cần biết vì sao bị hủy, và khách cần lời giải
// thích khi nhận thông báo.
//
// Từ PACKED trở đi KHÔNG hủy được bằng hàm này (quy tắc 8) — đã tốn công
// đóng gói và có thể đã bàn giao vận chuyển, nên cần quy trình riêng có
// tính chi phí.
func (f *FulfillmentOrder) Cancel(reason string, now time.Time) error {
	if reason == "" {
		return errors.New("order: hủy đơn thực hiện bắt buộc phải nêu lý do")
	}
	if err := f.transition(FOCancelled, now); err != nil {
		return err
	}
	f.cancelReason = reason
	f.cancelledAt = now
	return nil
}

func (f *FulfillmentOrder) transition(next FOStatus, now time.Time) error {
	if !f.status.canTransitionTo(next) {
		return ErrInvalidStatus
	}
	f.status = next
	if now.IsZero() {
		now = time.Now().UTC()
	}
	f.updatedAt = now
	return nil
}

// ---------------------------------------------------------------- Tách đơn

// SplitIntoFulfillmentOrders tách đơn hàng thành các đơn vị công việc.
//
// MỘT FulfillmentOrder cho MỖI SELLER (mục 3.2 của ADR-0007):
//
//	Giỏ hàng:
//	├── Áo own brand   (kho nền tảng, Hà Nội)
//	├── Giày Seller A  (kho seller A, TP.HCM)
//	└── Túi Seller B   (kho seller B, Đà Nẵng)
//
//	Ba món KHÔNG THỂ đóng chung một gói.
//
// Own brand cũng được tách như seller bình thường: nó là một seller nội bộ
// (INTERNAL), nên đơn lẫn own brand và hàng seller đi CHUNG một luồng.
//
// Mã FO đánh theo thứ tự: <order_number>-A, -B, -C. Seller thấy mã của
// mình mà không cần biết có bao nhiêu seller khác trong đơn.
func SplitIntoFulfillmentOrders(o *Order, now time.Time) ([]*FulfillmentOrder, error) {
	lines := o.ActiveLines()
	if len(lines) == 0 {
		return nil, ErrNoLines
	}

	// Gom dòng hàng theo seller, GIỮ THỨ TỰ xuất hiện để mã FO ổn định:
	// chạy lại phải ra cùng kết quả, không phụ thuộc thứ tự duyệt map.
	type group struct {
		sellerID   ids.ID
		lineIDs    []ids.ID
		subtotal   money.Money
		commission money.Money
	}
	var groups []*group
	index := map[ids.ID]*group{}

	for _, l := range lines {
		g, ok := index[l.SellerID()]
		if !ok {
			g = &group{
				sellerID:   l.SellerID(),
				subtotal:   money.Zero(o.Currency()),
				commission: money.Zero(o.Currency()),
			}
			index[l.SellerID()] = g
			groups = append(groups, g)
		}
		g.lineIDs = append(g.lineIDs, l.ID())
		g.subtotal, _ = g.subtotal.Add(l.LineTotal())
		g.commission, _ = g.commission.Add(l.CommissionAmount())
	}

	out := make([]*FulfillmentOrder, 0, len(groups))
	for i, g := range groups {
		fo, err := NewFulfillmentOrder(NewFulfillmentOrderParams{
			OrderID:          o.ID(),
			FONumber:         o.OrderNumber() + "-" + string(rune('A'+i)),
			SellerID:         g.sellerID,
			LineIDs:          g.lineIDs,
			Subtotal:         g.subtotal,
			CommissionAmount: g.commission,
			Now:              now,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, fo)
	}
	return out, nil
}
