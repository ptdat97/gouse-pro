// Package domain là mô hình nghiệp vụ của việc trả hàng.
//
// # Vì sao thư mục tên `returns` chứ không phải `return`
//
// `return` là từ khóa của Go, không dùng làm tên gói được. Tài liệu kiến
// trúc gọi module này là `return`; ở mã nguồn nó là `returns`.
package domain

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// TxFunc chạy trong giao dịch mà kho lưu trữ đang mở.
//
// Ngữ cảnh truyền vào MANG giao dịch đó, nên bên trong ghi được event vào
// outbox cùng lúc với yêu cầu trả hàng. Hai giao dịch tách rời nghĩa là có
// thể có yêu cầu mà không có tín hiệu, hoặc ngược lại.
type TxFunc func(ctx context.Context) error

var (
	ErrNotFound      = errors.New("returns: không tìm thấy yêu cầu trả hàng")
	ErrInvalidStatus = errors.New("returns: chuyển trạng thái không hợp lệ")

	// ErrReturnWindowClosed khi yêu cầu đến sau hạn đổi trả.
	//
	// Mốc tính từ lúc đơn GIAO XONG, độ dài lấy từ
	// `returns.window_hours` — cùng con số mà `fulfillment` dùng để biết
	// khi nào chuyển tiền cho nhà bán. Quá hạn mà vẫn cho trả nghĩa là
	// nền tảng hoàn tiền sau khi nhà bán đã rút được.
	ErrReturnWindowClosed = errors.New("returns: đã quá hạn đổi trả")
	ErrNoLines            = errors.New("returns: yêu cầu trả hàng phải có ít nhất một dòng")
	ErrInvalidReason      = errors.New("returns: lý do trả hàng không hợp lệ")
	ErrMissingReason      = errors.New("returns: phải nêu lý do từ chối")
	ErrVersionConflict    = errors.New("returns: yêu cầu vừa bị thay đổi, hãy đọc lại")
	ErrDuplicateLine      = errors.New("returns: dòng hàng này đã có yêu cầu trả")
	ErrQuantityExceeded   = errors.New("returns: số lượng xin trả vượt số đã mua")
	ErrGoodsNotReceived   = errors.New("returns: chưa nhận được hàng, không kiểm định được")
	ErrAlreadyInspected   = errors.New("returns: dòng hàng này đã kiểm định rồi")
	ErrMissingFaultReason = errors.New("returns: loại hàng phải nêu lý do")

	// ErrDiscountNotAllocated là HÀNG RÀO CHỐNG HOÀN THỪA.
	//
	// Đơn có giảm giá ở CẤP ĐƠN mà không dòng nào mang khoản điều chỉnh
	// tương ứng nghĩa là phần giảm chưa được phân bổ xuống dòng. Hoàn theo
	// giá dòng khi đó là hoàn nhiều hơn số khách đã trả.
	//
	// Thà TỪ CHỐI và bắt người vận hành xử lý tay, còn hơn âm thầm trả ra
	// số tiền sai. docs/07-workflows/return.md mục 5 gọi đây là điểm dễ
	// sai nhất của cả luồng.
	ErrDiscountNotAllocated = errors.New(
		"returns: đơn có giảm giá chưa phân bổ xuống dòng hàng — " +
			"không tính được số tiền thực trả, từ chối hoàn tự động")
)

// ReasonCode là lý do trả hàng, CHUẨN HÓA.
//
// Lý do quyết định DÒNG TIỀN và HÀNH ĐỘNG KHẮC PHỤC — hàng lỗi thì truy
// vết lô sản xuất, sai size thì sửa bảng size, hỏng khi vận chuyển thì
// khiếu nại đối tác. Văn bản tự do không phân tích được.
type ReasonCode string

const (
	ReasonSizeTooSmall     ReasonCode = "SIZE_TOO_SMALL"
	ReasonSizeTooLarge     ReasonCode = "SIZE_TOO_LARGE"
	ReasonNotAsDescribed   ReasonCode = "NOT_AS_DESCRIBED"
	ReasonColorMismatch    ReasonCode = "COLOR_DIFFERENT"
	ReasonQualityIssue     ReasonCode = "QUALITY_ISSUE"
	ReasonDefective        ReasonCode = "DEFECTIVE"
	ReasonWrongItem        ReasonCode = "WRONG_ITEM_SENT"
	ReasonDamagedInTransit ReasonCode = "DAMAGED_IN_TRANSIT"
	ReasonChangedMind      ReasonCode = "CHANGED_MIND"
	ReasonLateDelivery     ReasonCode = "LATE_DELIVERY"
)

var validReasons = map[ReasonCode]bool{
	ReasonSizeTooSmall: true, ReasonSizeTooLarge: true,
	ReasonNotAsDescribed: true, ReasonColorMismatch: true,
	ReasonQualityIssue: true, ReasonDefective: true,
	ReasonWrongItem: true, ReasonDamagedInTransit: true,
	ReasonChangedMind: true, ReasonLateDelivery: true,
}

func (l ReasonCode) IsValid() bool { return validReasons[l] }

// IsSellerFault cho biết lý do này quy trách nhiệm cho bên bán.
//
// Dùng để quyết ai chịu phí vận chuyển chiều về. Khách đổi ý thì khách
// chịu; hàng lỗi hay giao sai thì không.
func (l ReasonCode) IsSellerFault() bool {
	switch l {
	case ReasonDefective, ReasonWrongItem, ReasonNotAsDescribed, ReasonColorMismatch, ReasonQualityIssue:
		return true
	}
	return false
}

// Status là trạng thái một yêu cầu trả hàng.
type Status string

const (
	StatusRequested Status = "REQUESTED"
	StatusApproved  Status = "APPROVED"
	StatusRejected  Status = "REJECTED"
	StatusReceived  Status = "RECEIVED"
	StatusRefunded  Status = "REFUNDED"
	StatusCancelled Status = "CANCELLED"
)

// IsFinal cho biết trạng thái đã là cuối, không đổi được nữa.
func (t Status) IsFinal() bool {
	return t == StatusRejected || t == StatusRefunded || t == StatusCancelled
}

// Lines là một dòng hàng xin trả.
type Lines struct {
	ID          ids.ID
	OrderLineID ids.ID
	SKUID       ids.ID
	Quantity    int

	// RefundAmount là phần tiền của dòng này, ĐÓNG BĂNG lúc tạo yêu cầu.
	//
	// Đóng băng chứ không tính lại lúc hoàn: giữa lúc khách xin trả và lúc
	// hàng về kho, giá có thể đã đổi và khuyến mãi có thể đã kết thúc.
	RefundAmount money.Money

	// ReasonCode là lý do trả CỦA DÒNG NÀY.
	//
	// Khách trả hai món trong một đơn vì hai lý do khác nhau là chuyện
	// thường — cái áo chật, cái quần lỗi đường may. Mỗi lý do dẫn tới một
	// hành động khắc phục khác nhau, nên gộp chúng lại là mất cả hai.
	ReasonCode ReasonCode

	// Detail là mô tả thêm của khách cho dòng này.
	Detail string

	// Inspection là kết quả kiểm định hàng hoàn: PENDING, PASSED, FAILED.
	//
	// Hàng hoàn về kho nằm ở Returned và KHÔNG BAO GIỜ tự động vào
	// Available — bước này quyết định nó đi đâu.
	Inspection InspectionOutcome

	// InspectionNote BẮT BUỘC khi loại hàng: lý do loại là đầu vào cho
	// việc làm việc với nhà cung cấp và quyết ai chịu chi phí.
	InspectionNote string

	InspectedAt time.Time
}

// InspectionOutcome là kết quả kiểm định một dòng hàng hoàn.
type InspectionOutcome string

const (
	InspectionPending InspectionOutcome = "PENDING"
	InspectionPassed  InspectionOutcome = "PASSED"
	InspectionFailed  InspectionOutcome = "FAILED"
)

// ReturnRequest là một yêu cầu trả hàng.
type ReturnRequest struct {
	id         ids.ID
	orderID    ids.ID
	sellerID   ids.ID
	customerID ids.ID

	status       Status
	reason       ReasonCode
	note         string
	rejectReason string

	lines        []Lines
	refundAmount money.Money

	requestedAt time.Time
	decidedAt   time.Time
	receivedAt  time.Time
	refundedAt  time.Time

	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// NewParams là dữ liệu tạo một yêu cầu trả hàng.
type NewParams struct {
	OrderID    ids.ID
	SellerID   ids.ID
	CustomerID ids.ID
	Note       string
	Lines      []Lines
	Now        time.Time
}

// New dựng một yêu cầu trả hàng mới ở trạng thái REQUESTED.
func New(p NewParams) (*ReturnRequest, error) {
	if len(p.Lines) == 0 {
		return nil, ErrNoLines
	}
	for _, d := range p.Lines {
		if !d.ReasonCode.IsValid() {
			return nil, ErrInvalidReason
		}
	}

	// Tổng tiền hoàn = tổng các dòng. Cộng ở đây, một chỗ duy nhất, thay
	// vì để bên gọi truyền vào — hai nguồn cho cùng một con số tiền sớm
	// muộn sẽ lệch nhau.
	tong := p.Lines[0].RefundAmount
	seen := map[ids.ID]bool{p.Lines[0].OrderLineID: true}
	for _, d := range p.Lines[1:] {
		if seen[d.OrderLineID] {
			return nil, ErrDuplicateLine
		}
		seen[d.OrderLineID] = true

		sum, err := tong.Add(d.RefundAmount)
		if err != nil {
			return nil, err
		}
		tong = sum
	}

	return &ReturnRequest{
		id:         ids.MustNew(ids.PrefixReturnRequest),
		orderID:    p.OrderID,
		sellerID:   p.SellerID,
		customerID: p.CustomerID,
		status:     StatusRequested,
		// Lý do CHÍNH là lý do của dòng đầu — chỉ để lọc nhanh danh sách.
		// Lý do đầy đủ nằm ở từng dòng.
		reason:       p.Lines[0].ReasonCode,
		note:         strings.TrimSpace(p.Note),
		lines:        p.Lines,
		refundAmount: tong,
		requestedAt:  p.Now,
		createdAt:    p.Now,
		updatedAt:    p.Now,
	}, nil
}

// Approve chấp nhận yêu cầu trả hàng.
func (y *ReturnRequest) Approve(now time.Time) error {
	if y.status != StatusRequested {
		return ErrInvalidStatus
	}
	y.status = StatusApproved
	y.decidedAt = now
	y.touch(now)
	return nil
}

// TuChoi từ chối yêu cầu, BẮT BUỘC nêu lý do.
//
// Khách cần biết vì sao để quyết định làm gì tiếp — khiếu nại, gửi thêm
// ảnh, hay chấp nhận. Từ chối không lý do biến mọi trường hợp thành khiếu nại.
func (y *ReturnRequest) TuChoi(reason string, now time.Time) error {
	if y.status != StatusRequested {
		return ErrInvalidStatus
	}
	if strings.TrimSpace(reason) == "" {
		return ErrMissingReason
	}
	y.status = StatusRejected
	y.rejectReason = strings.TrimSpace(reason)
	y.decidedAt = now
	y.touch(now)
	return nil
}

// GoodsReceived ghi nhận hàng đã về kho.
//
// KHÔNG cộng vào tồn khả dụng — đó là việc của bước kiểm định, và quy tắc
// bắt buộc là hàng hoàn không bao giờ tự động thành Available. Vi phạm nó
// nghĩa là bán lại hàng hỏng cho khách khác.
func (y *ReturnRequest) GoodsReceived(now time.Time) error {
	if y.status != StatusApproved {
		return ErrInvalidStatus
	}
	y.status = StatusReceived
	y.receivedAt = now
	y.touch(now)
	return nil
}

// Refunded ghi nhận đã hoàn tiền.
func (y *ReturnRequest) Refunded(now time.Time) error {
	if y.status != StatusReceived {
		return ErrInvalidStatus
	}
	y.status = StatusRefunded
	y.refundedAt = now
	y.touch(now)
	return nil
}

// Cancel để khách rút yêu cầu khi chưa ai xử lý.
func (y *ReturnRequest) Cancel(now time.Time) error {
	if y.status != StatusRequested {
		return ErrInvalidStatus
	}
	y.status = StatusCancelled
	y.touch(now)
	return nil
}

func (y *ReturnRequest) touch(now time.Time) { y.updatedAt = now }

func (y *ReturnRequest) ID() ids.ID                { return y.id }
func (y *ReturnRequest) OrderID() ids.ID           { return y.orderID }
func (y *ReturnRequest) SellerID() ids.ID          { return y.sellerID }
func (y *ReturnRequest) CustomerID() ids.ID        { return y.customerID }
func (y *ReturnRequest) Status() Status            { return y.status }
func (y *ReturnRequest) ReasonCode() ReasonCode    { return y.reason }
func (y *ReturnRequest) Note() string              { return y.note }
func (y *ReturnRequest) RejectReason() string      { return y.rejectReason }
func (y *ReturnRequest) RefundAmount() money.Money { return y.refundAmount }
func (y *ReturnRequest) RequestedAt() time.Time    { return y.requestedAt }
func (y *ReturnRequest) DecidedAt() time.Time      { return y.decidedAt }
func (y *ReturnRequest) ReceivedAt() time.Time     { return y.receivedAt }
func (y *ReturnRequest) RefundedAt() time.Time     { return y.refundedAt }
func (y *ReturnRequest) Version() int64            { return y.version }
func (y *ReturnRequest) CreatedAt() time.Time      { return y.createdAt }
func (y *ReturnRequest) UpdatedAt() time.Time      { return y.updatedAt }

// Lines trả bản sao danh sách dòng hàng.
func (y *ReturnRequest) Lines() []Lines {
	return append([]Lines(nil), y.lines...)
}

// KhoiPhucParams dựng lại từ kho lưu trữ.
type KhoiPhucParams struct {
	ID           ids.ID
	OrderID      ids.ID
	SellerID     ids.ID
	CustomerID   ids.ID
	Status       Status
	ReasonCode   ReasonCode
	Note         string
	RejectReason string
	Lines        []Lines
	RefundAmount money.Money

	RequestedAt time.Time
	DecidedAt   time.Time
	ReceivedAt  time.Time
	RefundedAt  time.Time

	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KhoiPhuc dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func KhoiPhuc(p KhoiPhucParams) *ReturnRequest {
	return &ReturnRequest{
		id: p.ID, orderID: p.OrderID, sellerID: p.SellerID,
		customerID: p.CustomerID, status: p.Status, reason: p.ReasonCode,
		note: p.Note, rejectReason: p.RejectReason,
		lines: p.Lines, refundAmount: p.RefundAmount,
		requestedAt: p.RequestedAt, decidedAt: p.DecidedAt,
		receivedAt: p.ReceivedAt, refundedAt: p.RefundedAt,
		version: p.Version, createdAt: p.CreatedAt, updatedAt: p.UpdatedAt,
	}
}

// RecordInspection ghi kết quả kiểm định cho một dòng hàng.
//
// # Vì sao chỉ cho kiểm sau khi ĐÃ NHẬN hàng
//
// Kiểm định là nhìn vào món hàng thật. Ghi kết quả trước khi hàng về là
// ghi một điều chưa ai biết — và nó sẽ đẩy hàng vào Available trong khi
// kho chưa có gì.
func (y *ReturnRequest) RecordInspection(
	orderLineID ids.ID, dat bool, note string, now time.Time,
) error {
	if y.status != StatusReceived && y.status != StatusRefunded {
		return ErrGoodsNotReceived
	}
	if !dat && strings.TrimSpace(note) == "" {
		return ErrMissingFaultReason
	}

	for i := range y.lines {
		if y.lines[i].OrderLineID != orderLineID {
			continue
		}
		if y.lines[i].Inspection != InspectionPending && y.lines[i].Inspection != "" {
			// Kiểm hai lần nghĩa là hàng vào Available hai lần — tồn kho
			// tăng thêm số hàng không có thật.
			return ErrAlreadyInspected
		}

		y.lines[i].Inspection = InspectionFailed
		if dat {
			y.lines[i].Inspection = InspectionPassed
		}
		y.lines[i].InspectionNote = strings.TrimSpace(note)
		y.lines[i].InspectedAt = now
		y.touch(now)
		return nil
	}
	return ErrNotFound
}

// AwaitingInspection cho biết còn dòng nào chưa kiểm không.
func (y *ReturnRequest) AwaitingInspection() bool {
	for _, d := range y.lines {
		if d.Inspection == InspectionPending || d.Inspection == "" {
			return true
		}
	}
	return false
}
