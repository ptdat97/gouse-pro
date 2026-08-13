// Package domain chứa mô hình nghiệp vụ của module seller.
//
// Module này sở hữu DANH TÍNH và CHÍNH SÁCH của nhà bán. Nó KHÔNG sở hữu
// offer, đơn hàng, hay tiền.
//
// Muốn biết seller còn bao nhiêu tiền → gọi payment.GetBalance(). Không lưu
// trùng: hai nơi cùng lưu một sự thật thì sớm muộn chúng lệch nhau, và khi
// lệch thì không biết bên nào đúng.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

var (
	ErrEmptyName     = errors.New("seller: tên nhà bán không được rỗng")
	ErrEmptySlug     = errors.New("seller: slug không được rỗng")
	ErrInvalidStatus = errors.New("seller: chuyển trạng thái không hợp lệ")
	ErrNoBankAccount = errors.New("seller: nhà bán hoạt động phải có tài khoản ngân hàng đã xác minh")
	ErrMissingReason = errors.New("seller: phải nêu lý do")
	ErrNotFound      = errors.New("seller: không tìm thấy")
	ErrSlugTaken     = errors.New("seller: slug đã được sử dụng")
)

// SellerType phân loại nhà bán.
//
// Bốn loại khác nhau ở CHÍNH SÁCH, không ở CẤU TRÚC dữ liệu (mục 4 của
// đặc tả). Cùng một aggregate, phân biệt bằng trường này.
//
// Lý do: seller cá nhân có thể phát triển thành local brand. Nếu bốn loại
// là bốn bảng, nâng cấp là di trú dữ liệu; một bảng thì chỉ là đổi thuộc tính.
type SellerType string

const (
	// SellerInternal là OWN BRAND của nền tảng.
	//
	// Quyết định thiết kế quan trọng (mục 3): own brand là một seller nội
	// bộ, KHÔNG phải đường đi riêng. Nhờ vậy đơn hàng lẫn own brand và
	// hàng seller đi CHUNG MỘT LUỒNG, không cần logic đặc biệt.
	//
	// Khác biệt bản chất duy nhất nằm ở tầng ledger: đơn own brand ghi
	// doanh thu toàn phần + giá vốn; đơn marketplace ghi hoa hồng.
	SellerInternal SellerType = "INTERNAL"

	SellerIndividual SellerType = "INDIVIDUAL"
	SellerBusiness   SellerType = "BUSINESS"
	SellerLocalBrand SellerType = "LOCAL_BRAND"
	SellerStrategic  SellerType = "STRATEGIC_PARTNER"
)

func (t SellerType) valid() bool {
	switch t {
	case SellerInternal, SellerIndividual, SellerBusiness, SellerLocalBrand, SellerStrategic:
		return true
	}
	return false
}

// Status là trạng thái vòng đời nhà bán (mục 5 của đặc tả).
type Status string

const (
	StatusApplied       Status = "APPLIED"
	StatusPendingReview Status = "PENDING_REVIEW"
	StatusApproved      Status = "APPROVED"
	StatusActive        Status = "ACTIVE"
	StatusRejected      Status = "REJECTED"
	StatusSuspended     Status = "SUSPENDED"
	StatusOnVacation    Status = "ON_VACATION"
	StatusTerminated    Status = "TERMINATED"
)

// canTransitionTo mã hóa vòng đời ở mục 5.
//
// Đặt luật chuyển trạng thái Ở ĐÂY, không rải rác trong use case — nếu rải
// rác, mỗi chỗ sẽ quên một nhánh và trạng thái sẽ trôi.
func (s Status) canTransitionTo(next Status) bool {
	switch s {
	case StatusApplied:
		return next == StatusPendingReview || next == StatusRejected
	case StatusPendingReview:
		return next == StatusApproved || next == StatusRejected
	case StatusApproved:
		// Duyệt xong mới kích hoạt được — và việc kích hoạt còn cần tài
		// khoản ngân hàng đã xác minh (quy tắc 1).
		return next == StatusActive || next == StatusTerminated
	case StatusActive:
		return next == StatusSuspended || next == StatusOnVacation || next == StatusTerminated
	case StatusSuspended, StatusOnVacation:
		return next == StatusActive || next == StatusTerminated
	case StatusRejected:
		// Bị từ chối có thể nộp lại hồ sơ.
		return next == StatusPendingReview
	case StatusTerminated:
		// Chấm dứt là trạng thái CUỐI. Không có đường quay lại: đã đối
		// soát lần cuối và chi trả số dư, mở lại sẽ làm hỏng sổ sách.
		return false
	}
	return false
}

// HidesOffers cho biết trạng thái này có làm ẩn toàn bộ offer không.
//
// Quy tắc 4 của marketplace: seller bị đình chỉ → mọi offer ẩn.
//
// LƯU Ý QUAN TRỌNG (mục 5): ẩn offer KHÔNG phải hủy đơn. Đình chỉ seller
// không được hủy đơn hàng khách đã trả tiền — phải để seller hoàn tất,
// hoặc nền tảng hủy có kiểm soát và hoàn tiền khách.
func (s Status) HidesOffers() bool {
	return s == StatusSuspended || s == StatusOnVacation || s == StatusTerminated
}

// Seller là aggregate root — danh tính và chính sách của một nhà bán.
type Seller struct {
	id         ids.ID
	name       string
	slug       string
	sellerType SellerType
	status     Status

	// legalName và taxCode là thông tin pháp lý, cần cho hóa đơn và đối soát.
	legalName string
	taxCode   string

	email string
	phone string

	// commissionRate là tỷ lệ hoa hồng theo phần vạn.
	//
	// Seller INTERNAL (own brand) luôn bằng 0: nền tảng không thu hoa hồng
	// của chính mình. Doanh thu own brand ghi toàn phần ở tầng ledger.
	commissionRate types.BasisPoints

	// bankAccountVerified là điều kiện BẮT BUỘC để kích hoạt (quy tắc 1).
	//
	// Không có tài khoản đã xác minh thì không chi trả được, và seller sẽ
	// bán hàng rồi không nhận được tiền — tranh chấp không đáng có.
	bankAccountVerified bool

	// suspensionReason lưu lý do đình chỉ để trả lời seller khi họ hỏi.
	suspensionReason string

	approvedBy string
	approvedAt time.Time

	createdAt time.Time
	updatedAt time.Time
}

type NewSellerParams struct {
	Name           string
	Slug           string
	SellerType     SellerType
	LegalName      string
	TaxCode        string
	Email          string
	Phone          string
	CommissionRate types.BasisPoints
	Now            time.Time
}

// NewSeller tạo hồ sơ đăng ký nhà bán ở trạng thái APPLIED.
func NewSeller(p NewSellerParams) (*Seller, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	slug := strings.TrimSpace(p.Slug)
	if slug == "" {
		return nil, ErrEmptySlug
	}

	sellerType := p.SellerType
	if sellerType == "" {
		sellerType = SellerIndividual
	}
	if !sellerType.valid() {
		return nil, errors.New("seller: loại nhà bán không hợp lệ: " + string(sellerType))
	}

	rate := p.CommissionRate
	// Own brand KHÔNG chịu hoa hồng: nền tảng không thu của chính mình.
	if sellerType == SellerInternal {
		rate = types.BasisPoints{}
	}

	id, err := ids.New(ids.PrefixSeller)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Seller{
		id:             id,
		name:           name,
		slug:           slug,
		sellerType:     sellerType,
		status:         StatusApplied,
		legalName:      strings.TrimSpace(p.LegalName),
		taxCode:        strings.TrimSpace(p.TaxCode),
		email:          strings.TrimSpace(p.Email),
		phone:          strings.TrimSpace(p.Phone),
		commissionRate: rate,
		createdAt:      now,
		updatedAt:      now,
	}, nil
}

// RestoreSellerParams dựng lại từ kho lưu trữ.
type RestoreSellerParams struct {
	ID                  ids.ID
	Name                string
	Slug                string
	SellerType          SellerType
	Status              Status
	LegalName           string
	TaxCode             string
	Email               string
	Phone               string
	CommissionRate      types.BasisPoints
	BankAccountVerified bool
	SuspensionReason    string
	ApprovedBy          string
	ApprovedAt          time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// RestoreSeller dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreSeller(p RestoreSellerParams) *Seller {
	return &Seller{
		id:                  p.ID,
		name:                p.Name,
		slug:                p.Slug,
		sellerType:          p.SellerType,
		status:              p.Status,
		legalName:           p.LegalName,
		taxCode:             p.TaxCode,
		email:               p.Email,
		phone:               p.Phone,
		commissionRate:      p.CommissionRate,
		bankAccountVerified: p.BankAccountVerified,
		suspensionReason:    p.SuspensionReason,
		approvedBy:          p.ApprovedBy,
		approvedAt:          p.ApprovedAt,
		createdAt:           p.CreatedAt,
		updatedAt:           p.UpdatedAt,
	}
}

func (s *Seller) ID() ids.ID                        { return s.id }
func (s *Seller) Name() string                      { return s.name }
func (s *Seller) Slug() string                      { return s.slug }
func (s *Seller) Type() SellerType                  { return s.sellerType }
func (s *Seller) Status() Status                    { return s.status }
func (s *Seller) LegalName() string                 { return s.legalName }
func (s *Seller) TaxCode() string                   { return s.taxCode }
func (s *Seller) Email() string                     { return s.email }
func (s *Seller) Phone() string                     { return s.phone }
func (s *Seller) CommissionRate() types.BasisPoints { return s.commissionRate }
func (s *Seller) BankAccountVerified() bool         { return s.bankAccountVerified }
func (s *Seller) SuspensionReason() string          { return s.suspensionReason }
func (s *Seller) ApprovedBy() string                { return s.approvedBy }
func (s *Seller) ApprovedAt() time.Time             { return s.approvedAt }
func (s *Seller) CreatedAt() time.Time              { return s.createdAt }
func (s *Seller) UpdatedAt() time.Time              { return s.updatedAt }

// IsActive cho biết nhà bán có đang bán hàng được không.
func (s *Seller) IsActive() bool { return s.status == StatusActive }

// IsInternal cho biết đây có phải own brand của nền tảng không.
func (s *Seller) IsInternal() bool { return s.sellerType == SellerInternal }

// OffersHidden cho biết offer của nhà bán này có đang bị ẩn không.
func (s *Seller) OffersHidden() bool { return s.status.HidesOffers() }

// ---------------------------------------------------------------- Hành vi

// SubmitForReview nộp hồ sơ đi duyệt.
func (s *Seller) SubmitForReview(now time.Time) error {
	return s.transition(StatusPendingReview, now)
}

// Approve duyệt hồ sơ nhà bán.
func (s *Seller) Approve(by string, now time.Time) error {
	by = strings.TrimSpace(by)
	if by == "" {
		return errors.New("seller: phải ghi người duyệt")
	}
	if err := s.transition(StatusApproved, now); err != nil {
		return err
	}
	s.approvedBy = by
	s.approvedAt = now
	s.suspensionReason = ""
	return nil
}

// Reject từ chối hồ sơ kèm lý do.
//
// Lý do là BẮT BUỘC: "hồ sơ bị từ chối" không cho người nộp biết phải sửa
// gì, dẫn tới nộp lại y nguyên và tốn thêm một vòng duyệt.
func (s *Seller) Reject(reason string, now time.Time) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrMissingReason
	}
	if err := s.transition(StatusRejected, now); err != nil {
		return err
	}
	s.suspensionReason = reason
	return nil
}

// Activate kích hoạt nhà bán để bắt đầu bán hàng.
//
// QUY TẮC 1 (mục 10): nhà bán ACTIVE phải có tài khoản ngân hàng đã xác minh.
//
// Không có thì họ bán được hàng nhưng không nhận được tiền — tranh chấp
// không đáng có, và nền tảng giữ tiền hộ mà không biết trả về đâu.
//
// Seller INTERNAL được miễn: own brand nhận tiền qua sổ cái nội bộ, không
// qua chuyển khoản ngân hàng.
func (s *Seller) Activate(now time.Time) error {
	if !s.IsInternal() && !s.bankAccountVerified {
		return ErrNoBankAccount
	}
	return s.transition(StatusActive, now)
}

// Suspend đình chỉ nhà bán.
//
// HỆ QUẢ: ẩn toàn bộ offer, KHÔNG hủy đơn đang xử lý, giữ payout.
//
// Điểm dễ sai (mục 5): đình chỉ không được hủy đơn hàng khách đã trả tiền.
// Phải để seller hoàn tất, hoặc nền tảng hủy có kiểm soát và hoàn tiền khách.
func (s *Seller) Suspend(reason string, now time.Time) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrMissingReason
	}
	if err := s.transition(StatusSuspended, now); err != nil {
		return err
	}
	s.suspensionReason = reason
	return nil
}

// Reactivate khôi phục nhà bán sau khi đình chỉ hoặc nghỉ bán.
func (s *Seller) Reactivate(now time.Time) error {
	if !s.IsInternal() && !s.bankAccountVerified {
		return ErrNoBankAccount
	}
	if err := s.transition(StatusActive, now); err != nil {
		return err
	}
	s.suspensionReason = ""
	return nil
}

// GoOnVacation chuyển sang chế độ nghỉ bán.
//
// Khác đình chỉ ở chỗ đây là lựa chọn CỦA SELLER, không phải hình phạt.
// Nhưng hệ quả với offer giống nhau: ẩn hết. Seller vẫn phải hoàn tất đơn
// đang có.
func (s *Seller) GoOnVacation(now time.Time) error {
	return s.transition(StatusOnVacation, now)
}

// Terminate chấm dứt hợp tác.
//
// Trạng thái CUỐI. Trước khi gọi, tầng gọi phải bảo đảm: đơn đang xử lý đã
// hoàn tất hoặc hủy có kiểm soát, đã đối soát lần cuối, đã chi trả số dư
// còn lại (quy tắc 6).
func (s *Seller) Terminate(reason string, now time.Time) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrMissingReason
	}
	if err := s.transition(StatusTerminated, now); err != nil {
		return err
	}
	s.suspensionReason = reason
	return nil
}

// VerifyBankAccount đánh dấu tài khoản ngân hàng đã xác minh.
//
// KHÔNG lưu số tài khoản ở đây — thông tin ngân hàng thuộc bảng riêng và
// không bao giờ được ghi log (xem internal/platform/logger).
func (s *Seller) VerifyBankAccount(now time.Time) {
	s.bankAccountVerified = true
	s.touch(now)
}

// SetCommissionRate đặt tỷ lệ hoa hồng.
func (s *Seller) SetCommissionRate(rate types.BasisPoints, now time.Time) error {
	if s.IsInternal() {
		return errors.New("seller: own brand không chịu hoa hồng")
	}
	s.commissionRate = rate
	s.touch(now)
	return nil
}

func (s *Seller) transition(next Status, now time.Time) error {
	if !s.status.canTransitionTo(next) {
		return ErrInvalidStatus
	}
	s.status = next
	s.touch(now)
	return nil
}

func (s *Seller) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.updatedAt = now
}
