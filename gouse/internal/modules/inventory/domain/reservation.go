package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrReservationExpired   = errors.New("inventory: giữ hàng đã hết hạn")
	ErrReservationNotActive = errors.New("inventory: giữ hàng không còn hiệu lực")
	ErrTooManyExtensions    = errors.New("inventory: đã gia hạn quá số lần cho phép")
	ErrInvalidTTL           = errors.New("inventory: thời hạn giữ hàng phải lớn hơn 0")
)

// DefaultTTL là thời hạn giữ hàng mặc định (mục 6.2 của đặc tả).
//
// 15 phút đủ để khách nhập thông tin thanh toán, và đủ ngắn để hàng không
// bị khóa lâu khi khách bỏ ngang.
const DefaultTTL = 15 * time.Minute

// MaxExtensions giới hạn số lần gia hạn.
//
// Không giới hạn thì một checkout bỏ dở có thể giữ hàng vô hạn bằng cách
// gia hạn liên tục — hàng không bao giờ quay lại kệ.
const MaxExtensions = 3

// ReservationStatus là trạng thái của một lần giữ hàng.
type ReservationStatus string

const (
	ReservationActive    ReservationStatus = "ACTIVE"
	ReservationConverted ReservationStatus = "CONVERTED"
	ReservationExpired   ReservationStatus = "EXPIRED"
	ReservationReleased  ReservationStatus = "RELEASED"
)

// IsFinal cho biết trạng thái đã kết thúc, không đổi được nữa.
func (s ReservationStatus) IsFinal() bool {
	return s == ReservationConverted || s == ReservationExpired || s == ReservationReleased
}

// Reservation là một lần giữ hàng tạm thời cho checkout.
//
// VÌ SAO CẦN (mục 6.1): khách vào checkout cần đảm bảo hàng còn khi họ
// nhập thông tin thanh toán — nhưng không được giữ vĩnh viễn nếu họ bỏ ngang.
//
// Đây là lý do MỌI reservation đều có `expiresAt`: quy tắc 5 (mục 12).
type Reservation struct {
	id              ids.ID
	inventoryItemID ids.ID

	// checkoutID là tham chiếu VƯỢT MODULE — chỉ giữ định danh, inventory
	// không biết checkout là gì.
	checkoutID ids.ID

	quantity   int
	expiresAt  time.Time
	status     ReservationStatus
	extensions int

	createdAt time.Time
	updatedAt time.Time
}

type NewReservationParams struct {
	InventoryItemID ids.ID
	CheckoutID      ids.ID
	Quantity        int
	TTL             time.Duration
	Now             time.Time
}

// NewReservation tạo một lần giữ hàng.
func NewReservation(p NewReservationParams) (*Reservation, error) {
	if p.InventoryItemID.IsZero() {
		return nil, errors.New("inventory: thiếu định danh bản ghi tồn kho")
	}
	if p.Quantity <= 0 {
		return nil, errors.New("inventory: số lượng giữ phải lớn hơn 0")
	}

	ttl := p.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < 0 {
		return nil, ErrInvalidTTL
	}

	id, err := ids.New(ids.PrefixReservation)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Reservation{
		id:              id,
		inventoryItemID: p.InventoryItemID,
		checkoutID:      p.CheckoutID,
		quantity:        p.Quantity,
		expiresAt:       now.Add(ttl),
		status:          ReservationActive,
		createdAt:       now,
		updatedAt:       now,
	}, nil
}

// RestoreReservationParams dựng lại từ kho lưu trữ.
type RestoreReservationParams struct {
	ID              ids.ID
	InventoryItemID ids.ID
	CheckoutID      ids.ID
	Quantity        int
	ExpiresAt       time.Time
	Status          ReservationStatus
	Extensions      int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RestoreReservation dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreReservation(p RestoreReservationParams) *Reservation {
	return &Reservation{
		id:              p.ID,
		inventoryItemID: p.InventoryItemID,
		checkoutID:      p.CheckoutID,
		quantity:        p.Quantity,
		expiresAt:       p.ExpiresAt,
		status:          p.Status,
		extensions:      p.Extensions,
		createdAt:       p.CreatedAt,
		updatedAt:       p.UpdatedAt,
	}
}

func (r *Reservation) ID() ids.ID                { return r.id }
func (r *Reservation) InventoryItemID() ids.ID   { return r.inventoryItemID }
func (r *Reservation) CheckoutID() ids.ID        { return r.checkoutID }
func (r *Reservation) Quantity() int             { return r.quantity }
func (r *Reservation) ExpiresAt() time.Time      { return r.expiresAt }
func (r *Reservation) Status() ReservationStatus { return r.status }
func (r *Reservation) Extensions() int           { return r.extensions }
func (r *Reservation) CreatedAt() time.Time      { return r.createdAt }
func (r *Reservation) UpdatedAt() time.Time      { return r.updatedAt }

// IsExpiredAt cho biết đã hết hạn tại thời điểm t chưa.
//
// Biên: hết hạn ĐÚNG lúc expiresAt được coi là ĐÃ hết hạn. Chọn hướng chặt
// hơn vì giữ hàng thêm một khoảnh khắc gây hại (khách khác không mua được)
// nhiều hơn là giải phóng sớm một khoảnh khắc.
func (r *Reservation) IsExpiredAt(t time.Time) bool {
	return !t.Before(r.expiresAt)
}

// IsActiveAt cho biết còn hiệu lực tại thời điểm t không.
func (r *Reservation) IsActiveAt(t time.Time) bool {
	return r.status == ReservationActive && !r.IsExpiredAt(t)
}

// Convert chuyển thành cam kết khi đơn được xác nhận.
func (r *Reservation) Convert(now time.Time) error {
	if r.status != ReservationActive {
		return ErrReservationNotActive
	}
	// Hết hạn rồi thì KHÔNG cam kết được: hàng có thể đã được giải phóng
	// và bán cho khách khác.
	if r.IsExpiredAt(now) {
		return ErrReservationExpired
	}
	r.status = ReservationConverted
	r.touch(now)
	return nil
}

// Release giải phóng khi khách hủy checkout.
func (r *Reservation) Release(now time.Time) error {
	if r.status.IsFinal() {
		return ErrReservationNotActive
	}
	r.status = ReservationReleased
	r.touch(now)
	return nil
}

// Expire đánh dấu hết hạn — do tiến trình nền gọi.
//
// KHÔNG kiểm tra thời gian ở đây: tiến trình nền đã lọc theo expires_at
// trong truy vấn. Kiểm tra lại sẽ tạo ra khoảng trống khi đồng hồ tiến
// trình và đồng hồ database lệch nhau vài mili-giây.
func (r *Reservation) Expire(now time.Time) error {
	if r.status.IsFinal() {
		return ErrReservationNotActive
	}
	r.status = ReservationExpired
	r.touch(now)
	return nil
}

// Extend gia hạn thêm thời gian.
//
// Dùng khi khách đang ở bước thanh toán và cần thêm thời gian (ví dụ đang
// chuyển khoản ngân hàng). Giới hạn số lần để tránh giữ hàng vô hạn.
func (r *Reservation) Extend(d time.Duration, now time.Time) error {
	if r.status != ReservationActive {
		return ErrReservationNotActive
	}
	if r.IsExpiredAt(now) {
		return ErrReservationExpired
	}
	if d <= 0 {
		return ErrInvalidTTL
	}
	if r.extensions >= MaxExtensions {
		return ErrTooManyExtensions
	}
	r.expiresAt = r.expiresAt.Add(d)
	r.extensions++
	r.touch(now)
	return nil
}

func (r *Reservation) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.updatedAt = now
}
