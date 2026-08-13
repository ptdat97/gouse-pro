package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// MovementType là loại biến động tồn kho.
type MovementType string

const (
	MovementReceive     MovementType = "RECEIVE"
	MovementReserve     MovementType = "RESERVE"
	MovementRelease     MovementType = "RELEASE"
	MovementCommit      MovementType = "COMMIT"
	MovementUncommit    MovementType = "UNCOMMIT"
	MovementShip        MovementType = "SHIP"
	MovementReturn      MovementType = "RETURN"
	MovementInspectPass MovementType = "INSPECT_PASS"
	MovementInspectFail MovementType = "INSPECT_FAIL"
	MovementDamage      MovementType = "DAMAGE"
	MovementTransferOut MovementType = "TRANSFER_OUT"
	MovementTransferIn  MovementType = "TRANSFER_IN"
	MovementAdjust      MovementType = "ADJUST"
)

// RequiresReason cho biết loại biến động này có bắt buộc nêu lý do không.
//
// Quy tắc 7 (mục 12): điều chỉnh thủ công phải có lý do và người thực hiện.
// Điều chỉnh không lý do là điểm mù trong kiểm toán — không phân biệt được
// sai sót kiểm kê với thất thoát.
func (t MovementType) RequiresReason() bool {
	return t == MovementAdjust || t == MovementDamage
}

// InventoryMovement là MỘT DÒNG trong nhật ký biến động — BẤT BIẾN.
//
// Quy tắc 4 (mục 12): mọi biến động ghi vào đây.
//
// Không có phương thức sửa đổi. Nhật ký cho phép:
//   - tái dựng trạng thái tại bất kỳ thời điểm nào
//   - điều tra khi có sai lệch kiểm kê
//   - trả lời "vì sao số lượng đổi" chứ không chỉ "số lượng bây giờ là bao nhiêu"
//
// Ở tầng database, bảng này có trigger chặn UPDATE/DELETE — cùng cách làm
// với sổ cái tài chính (ADR-0008) và lịch sử giá.
type InventoryMovement struct {
	id              ids.ID
	inventoryItemID ids.ID
	skuID           ids.ID

	movementType MovementType

	// quantity LUÔN dương. Hướng của biến động nằm ở movementType, không
	// nằm ở dấu của số — dấu âm rất dễ đọc nhầm khi cộng dồn báo cáo.
	quantity int

	// quantityAfter là số lượng khả dụng SAU biến động.
	//
	// Lưu lại để đối chiếu: nếu cộng dồn nhật ký ra kết quả khác cột này,
	// nghĩa là có biến động không được ghi — đúng thứ cần phát hiện.
	quantityAfter int

	reason      string
	performedBy ids.ID

	// referenceID trỏ tới thứ gây ra biến động (reservation, đơn hàng,
	// phiếu nhập). Tham chiếu vượt module — chỉ giữ định danh.
	referenceID ids.ID

	occurredAt time.Time
}

type NewMovementParams struct {
	InventoryItemID ids.ID
	SKUID           ids.ID
	Type            MovementType
	Quantity        int
	QuantityAfter   int
	Reason          string
	PerformedBy     ids.ID
	ReferenceID     ids.ID
	Now             time.Time
}

// NewMovement ghi một dòng nhật ký.
func NewMovement(p NewMovementParams) (*InventoryMovement, error) {
	if p.InventoryItemID.IsZero() {
		return nil, errors.New("inventory: thiếu định danh bản ghi tồn kho")
	}
	if p.Quantity <= 0 {
		return nil, errors.New("inventory: số lượng biến động phải lớn hơn 0")
	}
	if p.QuantityAfter < 0 {
		return nil, ErrNegativeQuantity
	}

	reason := strings.TrimSpace(p.Reason)
	if p.Type.RequiresReason() && reason == "" {
		return nil, errors.New("inventory: biến động " + string(p.Type) + " bắt buộc phải nêu lý do")
	}

	id, err := ids.New(ids.PrefixInventoryMovement)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &InventoryMovement{
		id:              id,
		inventoryItemID: p.InventoryItemID,
		skuID:           p.SKUID,
		movementType:    p.Type,
		quantity:        p.Quantity,
		quantityAfter:   p.QuantityAfter,
		reason:          reason,
		performedBy:     p.PerformedBy,
		referenceID:     p.ReferenceID,
		occurredAt:      now,
	}, nil
}

// RestoreMovementParams dựng lại từ kho lưu trữ.
type RestoreMovementParams struct {
	ID              ids.ID
	InventoryItemID ids.ID
	SKUID           ids.ID
	Type            MovementType
	Quantity        int
	QuantityAfter   int
	Reason          string
	PerformedBy     ids.ID
	ReferenceID     ids.ID
	OccurredAt      time.Time
}

// RestoreMovement dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreMovement(p RestoreMovementParams) *InventoryMovement {
	return &InventoryMovement{
		id:              p.ID,
		inventoryItemID: p.InventoryItemID,
		skuID:           p.SKUID,
		movementType:    p.Type,
		quantity:        p.Quantity,
		quantityAfter:   p.QuantityAfter,
		reason:          p.Reason,
		performedBy:     p.PerformedBy,
		referenceID:     p.ReferenceID,
		occurredAt:      p.OccurredAt,
	}
}

func (m *InventoryMovement) ID() ids.ID              { return m.id }
func (m *InventoryMovement) InventoryItemID() ids.ID { return m.inventoryItemID }
func (m *InventoryMovement) SKUID() ids.ID           { return m.skuID }
func (m *InventoryMovement) Type() MovementType      { return m.movementType }
func (m *InventoryMovement) Quantity() int           { return m.quantity }
func (m *InventoryMovement) QuantityAfter() int      { return m.quantityAfter }
func (m *InventoryMovement) Reason() string          { return m.reason }
func (m *InventoryMovement) PerformedBy() ids.ID     { return m.performedBy }
func (m *InventoryMovement) ReferenceID() ids.ID     { return m.referenceID }
func (m *InventoryMovement) OccurredAt() time.Time   { return m.occurredAt }
