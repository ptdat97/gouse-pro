package domain

import (
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// ConsentType là loại đồng ý.
type ConsentType string

const (
	ConsentMarketingEmail ConsentType = "MARKETING_EMAIL"
	ConsentMarketingSMS   ConsentType = "MARKETING_SMS"

	// ConsentDataProcessing là đồng ý xử lý dữ liệu cá nhân.
	ConsentDataProcessing ConsentType = "DATA_PROCESSING"

	// ConsentPersonalization là đồng ý dùng hành vi để cá nhân hóa.
	ConsentPersonalization ConsentType = "PERSONALIZATION"
)

// ValidConsentType kiểm tra loại đồng ý có hợp lệ không.
func ValidConsentType(t ConsentType) bool {
	switch t {
	case ConsentMarketingEmail, ConsentMarketingSMS,
		ConsentDataProcessing, ConsentPersonalization:
		return true
	}
	return false
}

// Consent là MỘT LẦN ghi nhận đồng ý hoặc rút lại đồng ý.
//
// # Đây là NHẬT KÝ CHỈ GHI THÊM, không phải trạng thái
//
// Một cột boolean trên bảng customer chỉ nói được trạng thái HIỆN TẠI.
// Nghĩa vụ pháp lý ở nhiều thị trường là chứng minh được khách đã đồng ý
// VÀO LÚC NÀO và Ở ĐÂU — nên mỗi lần đổi ý là một hàng mới, không sửa
// hàng cũ.
//
// Trạng thái hiện tại là hàng MỚI NHẤT của mỗi loại.
type Consent struct {
	id         ids.ID
	customerID ids.ID

	consentType ConsentType
	granted     bool

	// source là NƠI khách đồng ý: "checkout", "signup_form", "settings".
	//
	// Bắt buộc không rỗng: "khách đã đồng ý" mà không nói được ở đâu thì
	// không dùng được làm bằng chứng.
	source string

	// policyVersion là phiên bản điều khoản khách đã đọc.
	//
	// Điều khoản thay đổi theo thời gian. Không lưu phiên bản thì không
	// trả lời được "khách đồng ý với ĐIỀU GÌ" — chỉ biết là họ đã bấm.
	policyVersion string

	ipHash    string
	userAgent string

	recordedAt time.Time
}

// NewConsentParams là dữ liệu ghi nhận đồng ý.
type NewConsentParams struct {
	CustomerID    ids.ID
	Type          ConsentType
	Granted       bool
	Source        string
	PolicyVersion string
	IPHash        string
	UserAgent     string
	Now           time.Time
}

// NewConsent ghi nhận một lần đồng ý hoặc rút lại.
func NewConsent(p NewConsentParams) (*Consent, error) {
	if !ValidConsentType(p.Type) {
		return nil, ErrInvalidConsent
	}

	source := strings.TrimSpace(p.Source)
	if source == "" {
		return nil, ErrInvalidConsent
	}

	return &Consent{
		id:            ids.MustNew(ids.PrefixConsent),
		customerID:    p.CustomerID,
		consentType:   p.Type,
		granted:       p.Granted,
		source:        source,
		policyVersion: strings.TrimSpace(p.PolicyVersion),
		ipHash:        p.IPHash,
		userAgent:     p.UserAgent,
		recordedAt:    p.Now,
	}, nil
}

// RestoreConsentParams dựng lại bản ghi từ kho lưu trữ.
type RestoreConsentParams struct {
	ID            ids.ID
	CustomerID    ids.ID
	Type          ConsentType
	Granted       bool
	Source        string
	PolicyVersion string
	IPHash        string
	UserAgent     string
	RecordedAt    time.Time
}

func RestoreConsent(p RestoreConsentParams) *Consent {
	return &Consent{
		id:            p.ID,
		customerID:    p.CustomerID,
		consentType:   p.Type,
		granted:       p.Granted,
		source:        p.Source,
		policyVersion: p.PolicyVersion,
		ipHash:        p.IPHash,
		userAgent:     p.UserAgent,
		recordedAt:    p.RecordedAt,
	}
}

func (c *Consent) ID() ids.ID            { return c.id }
func (c *Consent) CustomerID() ids.ID    { return c.customerID }
func (c *Consent) Type() ConsentType     { return c.consentType }
func (c *Consent) Granted() bool         { return c.granted }
func (c *Consent) Source() string        { return c.source }
func (c *Consent) PolicyVersion() string { return c.policyVersion }
func (c *Consent) IPHash() string        { return c.ipHash }
func (c *Consent) UserAgent() string     { return c.userAgent }
func (c *Consent) RecordedAt() time.Time { return c.recordedAt }

// RequiresOptIn cho biết loại này có BẮT BUỘC phải đồng ý trước không.
//
// # Vì sao mặc định là KHÔNG ĐỒNG Ý
//
// Không tìm thấy bản ghi nào nghĩa là khách CHƯA đồng ý, không phải "chưa
// từ chối". Suy diễn ngược lại là gửi thư quảng cáo cho người chưa bao giờ
// bấm đồng ý — vi phạm pháp luật ở nhiều thị trường.
//
// DATA_PROCESSING là ngoại lệ: không có nó thì không xử lý được đơn hàng,
// nên nó được thu thập lúc đặt hàng chứ không phải tùy chọn.
func RequiresOptIn(t ConsentType) bool {
	return t == ConsentMarketingEmail || t == ConsentMarketingSMS
}
