package domain

import (
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// CommissionStatus là trạng thái của một khoản hoa hồng creator.
//
//	PENDING    đã quy công, CHƯA được chi — đơn còn trong hạn đổi trả
//	AVAILABLE  hết hạn đổi trả, vào được đợt chi trả
//	REVERSED   đã đảo vì đơn hủy hoặc hoàn hàng
type CommissionStatus string

const (
	CommissionPending   CommissionStatus = "PENDING"
	CommissionAvailable CommissionStatus = "AVAILABLE"
	CommissionReversed  CommissionStatus = "REVERSED"
)

// DefaultRate là tỷ lệ hoa hồng creator khi chưa có chiến dịch.
//
// # Vì sao một HẰNG SỐ trong miền này
//
// affiliate.md mục 5 nói tỷ lệ *"phải được ghi rõ trong Campaign, không mặc
// định"*. Module `campaign` là Phase 2 và chưa tồn tại.
//
// Ba lựa chọn, và vì sao chọn cái này:
//
//	hằng số ở đây          module affiliate tự quyết mặc định của mình.
//	                       KHÔNG dòng nào ngoài module này phải đổi.
//	khóa trong opsconfig   sổ đăng ký cấu hình là ĐÓNG (ADR-0015), nên
//	                       thêm khóa là sửa platform — một module extension
//	                       không nên bắt platform biết về hoa hồng creator
//	bảng riêng + màn hình  đúng đích, nhưng cần admin UI mà chưa ai yêu cầu
//
// Khi `campaign` ra đời, nó truyền tỷ lệ vào `NewAttributionParams.Rate` và
// hằng số này thành mặc định cho đường không qua chiến dịch. Không có gì
// phải xóa.
//
// 500 điểm cơ bản = 5%.
const DefaultRate = 500

// DefaultCostBearer: nền tảng chịu chi phí hoa hồng khi chưa có chiến dịch.
//
// affiliate.md mục 5: chiến dịch do nền tảng khởi xướng thì nền tảng chịu.
// Không có chiến dịch nào thì đây là trường hợp ấy.
//
// Đoán sang nhà bán sẽ là lấy tiền của người ngoài công ty — cùng lý do
// `checkout.ApplyDiscount` mặc định `CostBearerPlatform`.
const DefaultCostBearer = CostBearerPlatform

// CommissionRecord là TRẠNG THÁI của một khoản hoa hồng.
//
// Tách khỏi `Attribution` vì hai thứ có vòng đời khác nhau: quy công bất
// biến, còn khoản tiền thì đi qua các trạng thái và có thể bị đảo.
type CommissionRecord struct {
	id            ids.ID
	attributionID ids.ID
	creatorID     ids.ID

	amount money.Money
	status CommissionStatus

	reversedReason string
	reversedAt     time.Time

	createdAt time.Time
	updatedAt time.Time
}

// NewCommission dựng khoản hoa hồng từ một bản quy công.
//
// Số tiền LẤY TỪ `Attribution.HoaHong()`, không nhận từ bên gọi: tỷ lệ và
// tiền gốc đã đóng băng ở đó, và một con số tiền thứ hai tính ở chỗ khác là
// một chỗ để lệch.
func NewCommission(a *Attribution, now time.Time) (*CommissionRecord, error) {
	id, err := ids.New(ids.PrefixCommissionRecord)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &CommissionRecord{
		id:            id,
		attributionID: a.ID(),
		creatorID:     a.CreatorID(),
		amount:        a.Commission(),

		// Bắt đầu ở CHỜ ĐẾN HẠN, không phải rút được (quy tắc 4).
		//
		// Hoa hồng chỉ rút được sau khi hết hạn đổi trả, giống số dư nhà
		// bán. Nhờ đó phần lớn hoàn hàng xảy ra TRƯỚC khi tiền được chi, và
		// việc đảo ngược không phải đòi lại tiền đã trả.
		status:    CommissionPending,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// MarkAvailable chuyển khoản hoa hồng sang trạng thái chi được.
//
// `returnWindowPassed` do bên gọi quyết định: hạn đổi trả là cấu hình nghiệp vụ
// mà module `fulfillment` sở hữu, và affiliate không được tự đoán nó.
func (c *CommissionRecord) MarkAvailable(returnWindowPassed bool, now time.Time) error {
	if c.status == CommissionReversed {
		return ErrAlreadyReversed
	}
	if !returnWindowPassed {
		return ErrReturnWindowOpen
	}
	if c.status == CommissionAvailable {
		return nil // idempotent
	}

	c.status = CommissionAvailable
	c.touch(now)
	return nil
}

// Dao đảo khoản hoa hồng khi đơn bị hủy hoặc hoàn hàng (quy tắc 5).
//
// `lyDo` BẮT BUỘC: một khoản tiền biến mất mà không nêu lý do là điểm mù
// trong kiểm toán — cùng quy tắc với bút toán điều chỉnh của payment.
//
// Idempotent: `order.cancelled` giao ít nhất một lần.
func (c *CommissionRecord) Reverse(reason string, now time.Time) error {
	if reason == "" {
		return ErrReverseReasonRequired
	}
	if c.status == CommissionReversed {
		return nil // idempotent
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.status = CommissionReversed
	c.reversedReason = reason
	c.reversedAt = now
	c.touch(now)
	return nil
}

func (c *CommissionRecord) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
}

// RestoreCommissionParams dựng lại từ kho lưu trữ.
type RestoreCommissionParams struct {
	ID             ids.ID
	AttributionID  ids.ID
	CreatorID      ids.ID
	Amount         money.Money
	Status         CommissionStatus
	ReversedReason string
	ReversedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func RestoreCommission(p RestoreCommissionParams) *CommissionRecord {
	return &CommissionRecord{
		id:             p.ID,
		attributionID:  p.AttributionID,
		creatorID:      p.CreatorID,
		amount:         p.Amount,
		status:         p.Status,
		reversedReason: p.ReversedReason,
		reversedAt:     p.ReversedAt,
		createdAt:      p.CreatedAt,
		updatedAt:      p.UpdatedAt,
	}
}

func (c *CommissionRecord) ID() ids.ID               { return c.id }
func (c *CommissionRecord) AttributionID() ids.ID    { return c.attributionID }
func (c *CommissionRecord) CreatorID() ids.ID        { return c.creatorID }
func (c *CommissionRecord) Amount() money.Money      { return c.amount }
func (c *CommissionRecord) Status() CommissionStatus { return c.status }
func (c *CommissionRecord) ReversedReason() string   { return c.reversedReason }
func (c *CommissionRecord) ReversedAt() time.Time    { return c.reversedAt }
func (c *CommissionRecord) CreatedAt() time.Time     { return c.createdAt }
func (c *CommissionRecord) UpdatedAt() time.Time     { return c.updatedAt }

// mustRate để infrastructure dựng lại tỷ lệ đã lưu.
//
// Giá trị trong database đã qua CHECK (0..10000) của migration, nên một giá
// trị ngoài khoảng chỉ có thể là dữ liệu bị sửa tay — thà panic còn hơn tính
// tiền bằng một tỷ lệ vô nghĩa.
func MustRate(bp int) types.BasisPoints {
	r, err := types.NewBasisPoints(int32(bp))
	if err != nil {
		panic(fmt.Sprintf("affiliate: tỷ lệ %d trong database không hợp lệ: %v", bp, err))
	}
	return r
}
