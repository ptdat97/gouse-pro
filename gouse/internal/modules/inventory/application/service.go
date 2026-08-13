// Package application chứa các use case của module inventory.
//
// Tầng này điều phối: gọi domain để áp dụng quy tắc, gọi repository để
// đọc/ghi, và XỬ LÝ THỬ LẠI khi có tranh chấp đồng thời.
package application

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock là đồng hồ thật, dùng ở production.
var SystemClock Clock = systemClock{}

// maxRetries là số lần thử lại khi XUNG ĐỘT PHIÊN BẢN (mục 5.4 của đặc tả).
//
// Chỉ thử lại xung đột phiên bản — lần sau có thể thắng. KHÔNG thử lại khi
// hết hàng: hàng không tự xuất hiện, thử lại chỉ lãng phí tài nguyên và
// làm khách chờ lâu hơn trước khi nhận câu trả lời "hết hàng".
const maxRetries = 3

// Service là tầng application của module inventory.
type Service struct {
	uow   domain.UnitOfWork
	repos domain.Repos
	clock Clock
}

// Deps gom các phụ thuộc.
type Deps struct {
	// UnitOfWork để chạy thao tác GHI trong giao dịch.
	UnitOfWork domain.UnitOfWork

	// Repos dùng cho thao tác CHỈ ĐỌC, không cần giao dịch.
	Repos domain.Repos

	Clock Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		uow:   d.UnitOfWork,
		repos: d.Repos,
		clock: clock,
	}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Truy vấn

// Availability là số lượng khả dụng của một SKU.
type Availability struct {
	SKUID     ids.ID
	Available int

	// InStock là câu trả lời rút gọn cho giao diện.
	InStock bool
}

// GetAvailability tra số lượng khả dụng của NHIỀU SKU trong một lần gọi.
//
// locationID rỗng = cộng gộp mọi địa điểm và mọi chủ sở hữu. Đây là con số
// khách nhìn thấy: họ không quan tâm hàng nằm ở kho nào.
func (s *Service) GetAvailability(
	ctx context.Context, skuIDs []ids.ID, locationID ids.ID,
) (map[ids.ID]Availability, error) {
	found, err := s.repos.Items.FindBySKUs(ctx, skuIDs, locationID)
	if err != nil {
		return nil, err
	}

	out := make(map[ids.ID]Availability, len(skuIDs))
	for _, skuID := range skuIDs {
		total := 0
		for _, item := range found[skuID] {
			total += item.Available()
		}
		out[skuID] = Availability{SKUID: skuID, Available: total, InStock: total > 0}
	}
	return out, nil
}

// GetItemsBySKUs trả các bản ghi tồn kho của nhiều SKU.
//
// Checkout cần hàm này: nó biết SKU khách mua, nhưng Reserve làm việc trên
// InventoryItem (một SKU có thể nằm ở nhiều kho). Không có hàm này thì
// checkout không giữ được hàng.
func (s *Service) GetItemsBySKUs(
	ctx context.Context, skuIDs []ids.ID, locationID ids.ID,
) (map[ids.ID][]*domain.InventoryItem, error) {
	return s.repos.Items.FindBySKUs(ctx, skuIDs, locationID)
}

func (s *Service) GetItem(ctx context.Context, id ids.ID) (*domain.InventoryItem, error) {
	return s.repos.Items.FindByID(ctx, id)
}

func (s *Service) GetItemByKey(ctx context.Context, key domain.ItemKey) (*domain.InventoryItem, error) {
	return s.repos.Items.FindByKey(ctx, key)
}

func (s *Service) ListByOwner(
	ctx context.Context, ownerID ids.ID, limit, offset int,
) ([]*domain.InventoryItem, error) {
	if ownerID.IsZero() {
		// BẢO MẬT: thiếu chủ sở hữu phải là LỖI, không được âm thầm trả về
		// tồn kho của mọi seller.
		return nil, errors.New("inventory: bắt buộc phải có định danh chủ sở hữu")
	}
	return s.repos.Items.FindByOwner(ctx, ownerID, limit, offset)
}

func (s *Service) GetMovements(
	ctx context.Context, itemID ids.ID, limit int,
) ([]*domain.InventoryMovement, error) {
	return s.repos.Movements.FindByItem(ctx, itemID, limit)
}

// ---------------------------------------------------------------- Nhập kho

// ReceiveInput là dữ liệu nhập hàng vào kho.
type ReceiveInput struct {
	Key         domain.ItemKey
	Quantity    int
	ReferenceID ids.ID
	PerformedBy ids.ID
	BatchID     ids.ID
}

// Receive nhập hàng vào kho, tạo bản ghi tồn kho nếu chưa có.
func (s *Service) Receive(ctx context.Context, in ReceiveInput) (*domain.InventoryItem, error) {
	var out *domain.InventoryItem

	err := s.withRetry(ctx, func(r domain.Repos) error {
		item, err := r.Items.FindByKey(ctx, in.Key)
		if errors.Is(err, domain.ErrNotFound) {
			// Chưa có bản ghi: tạo mới với số lượng RỖNG rồi mới nhập, để
			// lần nhập này có mặt trong nhật ký (quy tắc 4).
			item, err = domain.NewInventoryItem(domain.NewItemParams{
				SKUID:             in.Key.SKUID,
				LocationID:        in.Key.LocationID,
				OwnerID:           in.Key.OwnerID,
				ProductionBatchID: in.BatchID,
				Now:               s.clock.Now(),
			})
			if err != nil {
				return err
			}
			if err := r.Items.Create(ctx, item); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		return s.mutate(ctx, r, item, mutation{
			apply:       func(i *domain.InventoryItem, now time.Time) error { return i.Receive(in.Quantity, now) },
			movement:    domain.MovementReceive,
			quantity:    in.Quantity,
			referenceID: in.ReferenceID,
			performedBy: in.PerformedBy,
			result:      &out,
		})
	})
	return out, err
}

// ---------------------------------------------------------------- Giữ hàng

// ReserveInput là dữ liệu giữ hàng cho checkout.
type ReserveInput struct {
	ItemID     ids.ID
	CheckoutID ids.ID
	Quantity   int
	TTL        time.Duration
}

// Reserve giữ hàng tạm thời cho một checkout.
//
// Đây là use case NÓNG NHẤT của module — mọi lượt checkout đều đi qua đây,
// và trong live commerce hàng nghìn lượt đổ vào cùng một SKU.
//
// Giữ hàng và ghi nhật ký nằm trong MỘT giao dịch: số lượng đổi mà không
// có vết thì sai lệch sau này không truy được nguyên nhân.
func (s *Service) Reserve(ctx context.Context, in ReserveInput) (*domain.Reservation, error) {
	var res *domain.Reservation

	err := s.withRetry(ctx, func(r domain.Repos) error {
		item, err := r.Items.FindByID(ctx, in.ItemID)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		reservation, err := domain.NewReservation(domain.NewReservationParams{
			InventoryItemID: item.ID(),
			CheckoutID:      in.CheckoutID,
			Quantity:        in.Quantity,
			TTL:             in.TTL,
			Now:             now,
		})
		if err != nil {
			return err
		}

		var saved *domain.InventoryItem
		if err := s.mutate(ctx, r, item, mutation{
			apply:       func(i *domain.InventoryItem, t time.Time) error { return i.Reserve(in.Quantity, t) },
			movement:    domain.MovementReserve,
			quantity:    in.Quantity,
			referenceID: reservation.ID(),
			result:      &saved,
		}); err != nil {
			return err
		}

		if err := r.Reservations.Save(ctx, reservation); err != nil {
			return err
		}
		res = reservation
		return nil
	})
	return res, err
}

// Commit chuyển hàng đang giữ thành cam kết cho đơn đã xác nhận.
func (s *Service) Commit(ctx context.Context, reservationID ids.ID) error {
	return s.withRetry(ctx, func(r domain.Repos) error {
		reservation, err := r.Reservations.FindByID(ctx, reservationID)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		if err := reservation.Convert(now); err != nil {
			return err
		}

		item, err := r.Items.FindByID(ctx, reservation.InventoryItemID())
		if err != nil {
			return err
		}

		var saved *domain.InventoryItem
		if err := s.mutate(ctx, r, item, mutation{
			apply:       func(i *domain.InventoryItem, t time.Time) error { return i.Commit(reservation.Quantity(), t) },
			movement:    domain.MovementCommit,
			quantity:    reservation.Quantity(),
			referenceID: reservation.ID(),
			result:      &saved,
		}); err != nil {
			return err
		}

		return r.Reservations.Save(ctx, reservation)
	})
}

// Release giải phóng hàng đang giữ khi khách hủy checkout.
func (s *Service) Release(ctx context.Context, reservationID ids.ID) error {
	return s.releaseWith(ctx, reservationID, func(r *domain.Reservation, now time.Time) error {
		return r.Release(now)
	})
}

// ExpireReservations dọn các reservation quá hạn — do tiến trình nền gọi.
//
// Cơ chế này phải ĐÁNG TIN CẬY: nếu ngừng chạy, hàng bị khóa dần và cuối
// cùng không bán được gì (mục 6.3).
//
// Trả về số lượng đã dọn để tiến trình nền ghi log và giám sát.
func (s *Service) ExpireReservations(ctx context.Context, limit int) (int, error) {
	now := s.clock.Now()

	expired, err := s.repos.Reservations.FindExpired(ctx, now, limit)
	if err != nil {
		return 0, err
	}

	daDon := 0
	for _, r := range expired {
		// Dọn TỪNG CÁI trong giao dịch riêng: một reservation lỗi không
		// được chặn việc dọn những cái còn lại — nếu không, một bản ghi
		// hỏng sẽ làm kẹt toàn bộ cơ chế.
		if err := s.releaseWith(ctx, r.ID(), func(res *domain.Reservation, t time.Time) error {
			return res.Expire(t)
		}); err != nil {
			// Đã bị xử lý bởi lượt khác — bình thường khi có nhiều worker.
			if errors.Is(err, domain.ErrReservationNotActive) {
				continue
			}
			return daDon, err
		}
		daDon++
	}
	return daDon, nil
}

// CountExpiredPending đếm reservation quá hạn chưa dọn — chỉ báo giám sát.
//
// Cảnh báo khi > 100 (mục 13): con số tăng dần nghĩa là tiến trình dọn đã
// ngừng chạy và hàng đang bị khóa mà không ai biết.
func (s *Service) CountExpiredPending(ctx context.Context) (int, error) {
	return s.repos.Reservations.CountExpiredPending(ctx, s.clock.Now())
}

// ExtendReservation gia hạn thời gian giữ hàng.
func (s *Service) ExtendReservation(ctx context.Context, reservationID ids.ID, d time.Duration) error {
	return s.uow.Do(ctx, func(r domain.Repos) error {
		reservation, err := r.Reservations.FindByID(ctx, reservationID)
		if err != nil {
			return err
		}
		if err := reservation.Extend(d, s.clock.Now()); err != nil {
			return err
		}
		return r.Reservations.Save(ctx, reservation)
	})
}

// releaseWith giải phóng hàng đang giữ theo một cách kết thúc cụ thể.
func (s *Service) releaseWith(
	ctx context.Context, reservationID ids.ID,
	finish func(*domain.Reservation, time.Time) error,
) error {
	return s.withRetry(ctx, func(r domain.Repos) error {
		reservation, err := r.Reservations.FindByID(ctx, reservationID)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		if err := finish(reservation, now); err != nil {
			return err
		}

		item, err := r.Items.FindByID(ctx, reservation.InventoryItemID())
		if err != nil {
			return err
		}

		var saved *domain.InventoryItem
		if err := s.mutate(ctx, r, item, mutation{
			apply:       func(i *domain.InventoryItem, t time.Time) error { return i.Release(reservation.Quantity(), t) },
			movement:    domain.MovementRelease,
			quantity:    reservation.Quantity(),
			referenceID: reservation.ID(),
			result:      &saved,
		}); err != nil {
			return err
		}

		return r.Reservations.Save(ctx, reservation)
	})
}

// ---------------------------------------------------------------- Xuất kho

// Ship xuất hàng đã cam kết ra khỏi kho.
func (s *Service) Ship(ctx context.Context, itemID ids.ID, qty int, orderID ids.ID) error {
	return s.simpleChange(ctx, itemID, qty, domain.MovementShip, orderID, "", "",
		func(i *domain.InventoryItem, t time.Time) error { return i.Ship(qty, t) })
}

// ReceiveReturn nhận hàng khách trả về.
//
// Hàng vào trạng thái Returned, KHÔNG vào Available (quy tắc 3).
func (s *Service) ReceiveReturn(ctx context.Context, itemID ids.ID, qty int, returnID ids.ID) error {
	return s.simpleChange(ctx, itemID, qty, domain.MovementReturn, returnID, "", "",
		func(i *domain.InventoryItem, t time.Time) error { return i.ReceiveReturn(qty, t) })
}

// InspectReturn xử lý kết quả kiểm định hàng hoàn.
//
// passed = true  → Returned → Available (bán lại được)
// passed = false → Returned → Damaged (không bán được)
func (s *Service) InspectReturn(
	ctx context.Context, itemID ids.ID, qty int, passed bool, reason string, by ids.ID,
) error {
	if passed {
		return s.simpleChange(ctx, itemID, qty, domain.MovementInspectPass, "", reason, by,
			func(i *domain.InventoryItem, t time.Time) error { return i.InspectionPassed(qty, t) })
	}
	return s.simpleChange(ctx, itemID, qty, domain.MovementInspectFail, "", reason, by,
		func(i *domain.InventoryItem, t time.Time) error { return i.InspectionFailed(qty, t) })
}

// AdjustInput là dữ liệu điều chỉnh thủ công (kiểm kê).
type AdjustInput struct {
	ItemID      ids.ID
	Delta       int
	Reason      string
	PerformedBy ids.ID
}

// Adjust điều chỉnh số lượng thủ công sau kiểm kê.
//
// Quy tắc 7 (mục 12): PHẢI có lý do và người thực hiện.
//
// Cưỡng chế ở đây chứ không chỉ ở database: báo lỗi sớm với thông báo rõ
// ràng tốt hơn để câu INSERT thất bại với lỗi ràng buộc khó hiểu.
func (s *Service) Adjust(ctx context.Context, in AdjustInput) error {
	if in.Reason == "" {
		return errors.New("inventory: điều chỉnh thủ công bắt buộc phải nêu lý do")
	}
	if in.PerformedBy.IsZero() {
		return errors.New("inventory: điều chỉnh thủ công bắt buộc phải ghi người thực hiện")
	}

	// Nhật ký lưu số lượng LUÔN DƯƠNG; hướng nằm ở loại biến động.
	qty := in.Delta
	if qty < 0 {
		qty = -qty
	}

	return s.simpleChange(ctx, in.ItemID, qty, domain.MovementAdjust, "", in.Reason, in.PerformedBy,
		func(i *domain.InventoryItem, t time.Time) error { return i.AdjustAvailable(in.Delta, t) })
}

// simpleChange áp dụng một thay đổi số lượng kèm ghi nhật ký.
func (s *Service) simpleChange(
	ctx context.Context, itemID ids.ID, qty int,
	mType domain.MovementType, refID ids.ID, reason string, by ids.ID,
	apply func(*domain.InventoryItem, time.Time) error,
) error {
	return s.withRetry(ctx, func(r domain.Repos) error {
		item, err := r.Items.FindByID(ctx, itemID)
		if err != nil {
			return err
		}
		var saved *domain.InventoryItem
		return s.mutate(ctx, r, item, mutation{
			apply:       apply,
			movement:    mType,
			quantity:    qty,
			referenceID: refID,
			reason:      reason,
			performedBy: by,
			result:      &saved,
		})
	})
}

// ---------------------------------------------------------------- Nội bộ

// mutation mô tả một thay đổi số lượng kèm dòng nhật ký tương ứng.
type mutation struct {
	apply       func(*domain.InventoryItem, time.Time) error
	movement    domain.MovementType
	quantity    int
	referenceID ids.ID
	reason      string
	performedBy ids.ID
	result      **domain.InventoryItem
}

// mutate áp dụng thay đổi, ghi bằng khóa lạc quan, và ghi nhật ký.
//
// Ba việc này phải cùng thành công hoặc cùng thất bại — chúng chạy trong
// giao dịch do withRetry mở. Quy tắc 4: mọi biến động phải có vết.
func (s *Service) mutate(
	ctx context.Context, r domain.Repos, item *domain.InventoryItem, m mutation,
) error {
	// Giữ version ĐÃ ĐỌC trước khi sửa: đó là cái database dùng để phát
	// hiện có tiến trình khác chen vào.
	expectedVersion := item.Version()
	now := s.clock.Now()

	if err := m.apply(item, now); err != nil {
		return err
	}

	if err := r.Items.ApplyChange(ctx, item, expectedVersion); err != nil {
		return err
	}

	movement, err := domain.NewMovement(domain.NewMovementParams{
		InventoryItemID: item.ID(),
		SKUID:           item.SKUID(),
		Type:            m.movement,
		Quantity:        m.quantity,
		QuantityAfter:   item.Available(),
		Reason:          m.reason,
		PerformedBy:     m.performedBy,
		ReferenceID:     m.referenceID,
		Now:             now,
	})
	if err != nil {
		return err
	}
	if err := r.Movements.Append(ctx, movement); err != nil {
		return err
	}

	if m.result != nil {
		*m.result = item
	}
	return nil
}

// withRetry chạy fn trong giao dịch, THỬ LẠI khi xung đột phiên bản.
//
// PHÂN BIỆT HAI LOẠI LỖI (mục 5.4 của đặc tả):
//
//	Xung đột phiên bản → thử lại tối đa 3 lần, chờ ngẫu nhiên ngắn
//	Không đủ hàng      → KHÔNG thử lại, trả lỗi ngay
//
// Phân biệt này quan trọng: hàng không tự xuất hiện, nên thử lại khi hết
// hàng chỉ lãng phí tài nguyên và làm khách chờ lâu hơn trước khi nhận
// câu trả lời "hết hàng".
func (s *Service) withRetry(ctx context.Context, fn func(domain.Repos) error) error {
	var lastErr error

	for lan := 0; lan < maxRetries; lan++ {
		err := s.uow.Do(ctx, fn)
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrVersionConflict) {
			// Mọi lỗi khác — kể cả hết hàng — trả về NGAY.
			return err
		}

		lastErr = err

		// Chờ ngẫu nhiên tăng dần trước khi thử lại.
		//
		// Ngẫu nhiên là cần thiết: nếu mọi request xung đột cùng chờ đúng
		// một khoảng rồi cùng thử lại, chúng lại xung đột tiếp.
		if lan < maxRetries-1 {
			// Dùng bộ sinh TOÀN CỤC của math/rand/v2, an toàn khi nhiều
			// goroutine gọi cùng lúc.
			//
			// Một *rand.Rand dùng chung KHÔNG an toàn: nó giữ trạng thái
			// nội bộ và nhiều request tranh nhau ghi vào đó. Đây đúng là
			// hàm bị gọi song song nhiều nhất trong hệ thống — mỗi lần
			// tranh chấp tồn kho đều đi qua đây.
			delay := time.Duration(rand.IntN(10*(lan+1))+1) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return fmt.Errorf("%w: đã thử lại %d lần", lastErr, maxRetries)
}
