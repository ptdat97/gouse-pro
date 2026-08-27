package http

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/seller/application"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

// PublicHandler phục vụ endpoint tra hồ sơ nhà bán cho KHÁCH.
//
// # Vì sao tách hẳn khỏi Handler quản trị
//
// Không phải để cho gọn. Hai handler trả hai TẬP TRƯỜNG khác nhau, và cái
// tách chúng ra là ranh giới bảo mật: hồ sơ quản trị có tên pháp lý, mã
// số thuế, email, số điện thoại và TỶ LỆ HOA HỒNG — điều khoản thương mại
// giữa nền tảng và nhà bán, không phải việc của khách.
//
// Dùng chung một struct rồi lọc bằng `omitempty` là cách rò rỉ dữ liệu
// kinh điển: thêm một trường vào hồ sơ quản trị, quên cập nhật bộ lọc, và
// nó xuất hiện ở endpoint công khai mà không ai nhận ra. Ở đây trường công
// khai được LIỆT KÊ RA, nên thêm trường mới mặc định là KHÔNG lộ.
type PublicHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewPublicHandler(svc *application.Service, log *slog.Logger) *PublicHandler {
	return &PublicHandler{svc: svc, log: log}
}

// Register gắn route công khai.
//
// KHÔNG bọc Auth: khách chưa đăng nhập vẫn phải xem được mình đang mua của
// ai. Đây là điều kiện để trang so sánh nhà bán có nghĩa.
func (h *PublicHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/sellers", http.HandlerFunc(h.listByIDs))
}

// RegisterApply gắn endpoint ĐĂNG KÝ làm nhà bán.
//
// Tách khỏi Register vì nó CẦN đăng nhập, còn tra hồ sơ thì không. Bên gọi
// PHẢI bọc Auth và RequireIdempotencyKey.
func (h *PublicHandler) RegisterApply(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/sellers", http.HandlerFunc(h.apply))
}

// maxSellerIDs chặn một lời gọi kéo cả bảng nhà bán.
//
// 50 rộng hơn nhiều so với nhu cầu thật: một trang sản phẩm hiếm khi có
// quá chục nhà bán, một giỏ hàng cũng vậy. Endpoint công khai không giới
// hạn là một công cụ trích xuất dữ liệu miễn phí.
const maxSellerIDs = 50

// sellerRefJSON là góc nhìn CÔNG KHAI của một nhà bán.
//
// Khớp schema `SellerRef` trong đặc tả: "Thông tin seller hiển thị cho
// khách. Không chứa dữ liệu nội bộ."
type sellerRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// IsOfficial: own brand của nền tảng hoặc đối tác chính hãng.
	//
	// Khách cần nó để phân biệt hàng chính hãng với hàng của nhà bán lẻ —
	// với thời trang, đó là câu hỏi về hàng thật hay hàng giả.
	IsOfficial bool `json:"is_official"`
}

type sellersResponse struct {
	Data []sellerRefJSON `json:"data"`
}

// listByIDs phục vụ GET /api/v1/sellers?ids=... (operationId: lookupSellers).
//
// # Vì sao tra theo LÔ chứ không phải từng cái
//
// Trang chi tiết sản phẩm hiện N offer của N nhà bán. Một endpoint
// `/sellers/{id}` buộc trang gọi N lần — đúng vấn đề N+1 mà module cart
// đã tránh bằng bốn lượt gọi cố định (xem cart/lookup.go).
//
// # Vì sao trang tự ghép chứ không phải endpoint offer trả kèm
//
// "Việc ghép dữ liệu là của TRANG, không phải của ENDPOINT". Nhét hồ sơ
// nhà bán vào mỗi offer nghĩa là mọi lời gọi offer đều kéo theo nó, kể cả
// những lời gọi không hiển thị tên ai — và cùng một nhà bán bị lặp lại ở
// mỗi offer của họ.
func (h *PublicHandler) listByIDs(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("ids"))
	if raw == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"ids là tham số bắt buộc"))
		return
	}

	parts := strings.Split(raw, ",")
	if len(parts) > maxSellerIDs {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"tối đa 50 mã mỗi lần gọi"))
		return
	}

	// Mã sai định dạng bị BỎ QUA, không làm hỏng cả lời gọi.
	//
	// Trang đang hiển thị một danh sách; một mã hỏng không đáng để cả
	// danh sách trống. Nhà bán không tìm thấy cũng vậy — họ chỉ vắng mặt
	// trong kết quả, và bên gọi tự biết phải làm gì với phần thiếu.
	parsed := make([]ids.ID, 0, len(parts))
	seen := make(map[ids.ID]bool, len(parts))
	for _, p := range parts {
		id, err := ids.Parse(strings.TrimSpace(p), ids.PrefixSeller)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		parsed = append(parsed, id)
	}

	data := make([]sellerRefJSON, 0, len(parsed))
	if len(parsed) > 0 {
		found, err := h.svc.GetSellersByIDs(r.Context(), parsed)
		if err != nil {
			h.fail(w, r, apierror.From(err))
			return
		}

		// GIỮ ĐÚNG THỨ TỰ khách hỏi: bên gọi thường ghép kết quả vào một
		// danh sách đã sắp xếp sẵn, và thứ tự map trong Go là ngẫu nhiên.
		for _, id := range parsed {
			sel, ok := found[id]
			if !ok {
				continue
			}
			data = append(data, sellerRefJSON{
				ID:         sel.ID().String(),
				Name:       sel.Name(),
				IsOfficial: sel.Type() == domain.SellerInternal,
			})
		}
	}

	h.ok(w, r, sellersResponse{Data: data})
}

func (h *PublicHandler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *PublicHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// ---------------------------------------------------------------- Đăng ký

// applyRequest khớp `applyAsSeller` trong đặc tả.
type applyRequest struct {
	SellerType   string `json:"seller_type"`
	BusinessName string `json:"business_name"`
	TaxID        string `json:"tax_id"`
	ContactEmail string `json:"contact_email"`
	ContactPhone string `json:"contact_phone"`

	BankAccount struct {
		BankCode      string `json:"bank_code"`
		AccountNumber string `json:"account_number"`
		AccountHolder string `json:"account_holder"`
	} `json:"bank_account"`
}

// apply nhận hồ sơ đăng ký làm nhà bán.
//
// # Vì sao endpoint này nằm ở nhóm storefront
//
// Người nộp hồ sơ CHƯA phải nhà bán — họ là một khách hàng đã đăng nhập.
// Đặt nó sau `RequireRole("SELLER_OWNER")` thì chỉ nhà bán mới đăng ký
// được làm nhà bán, và không ai vào được vòng.
//
// # Số tài khoản ngân hàng
//
// Đi thẳng xuống kho lưu trữ để mã hóa. KHÔNG ghi log, KHÔNG trả lại
// trong response — response chỉ có hồ sơ công khai, và bốn số cuối là
// thứ duy nhất của tài khoản từng đi ra ngoài.
func (h *PublicHandler) apply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	thieu := map[string]string{
		"seller_type":                 req.SellerType,
		"business_name":               req.BusinessName,
		"contact_email":               req.ContactEmail,
		"contact_phone":               req.ContactPhone,
		"bank_account.bank_code":      req.BankAccount.BankCode,
		"bank_account.account_number": req.BankAccount.AccountNumber,
		"bank_account.account_holder": req.BankAccount.AccountHolder,
	}
	for ten, gt := range thieu {
		if strings.TrimSpace(gt) == "" {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				ten+" là trường bắt buộc"))
			return
		}
	}

	// INTERNAL là own brand của nền tảng, không ai tự đăng ký được.
	if strings.EqualFold(req.SellerType, "INTERNAL") {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"seller_type INTERNAL dành riêng cho gian hàng của nền tảng"))
		return
	}

	sel, err := h.svc.Apply(r.Context(), application.ApplyInput{
		Name:       strings.TrimSpace(req.BusinessName),
		Slug:       taoSlug(req.BusinessName),
		SellerType: domain.SellerType(strings.ToUpper(strings.TrimSpace(req.SellerType))),
		LegalName:  strings.TrimSpace(req.BusinessName),
		TaxCode:    strings.TrimSpace(req.TaxID),
		Email:      strings.TrimSpace(req.ContactEmail),
		Phone:      strings.TrimSpace(req.ContactPhone),
		BankAccount: domain.TaiKhoanNganHang{
			BankCode: strings.TrimSpace(req.BankAccount.BankCode),
			Holder:   strings.TrimSpace(req.BankAccount.AccountHolder),
			Last4:    privacy.BonSoCuoi(req.BankAccount.AccountNumber),
		},
		SoTaiKhoanDayDu: strings.TrimSpace(req.BankAccount.AccountNumber),
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	// 201, không phải 200: hồ sơ vừa được TẠO. Trả về góc nhìn CÔNG KHAI
	// — người vừa nộp không cần đọc lại thứ họ vừa gửi, và mọi trường thừa
	// ở đây là một đường rò dữ liệu mới.
	body := map[string]any{
		"seller": sellerRefJSON{
			ID:         sel.ID().String(),
			Name:       sel.Name(),
			IsOfficial: sel.IsInternal(),
		},
	}
	if err := apierror.WriteJSON(w, http.StatusCreated, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

// taoSlug dựng slug từ tên doanh nghiệp.
//
// Đặc tả không nhận slug từ client: để client tự đặt là mở đường cho một
// nhà bán chiếm slug trùng thương hiệu người khác.
func taoSlug(ten string) string {
	var b strings.Builder
	truocLaGach := true
	for _, r := range strings.ToLower(strings.TrimSpace(ten)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			truocLaGach = false
		default:
			if !truocLaGach {
				b.WriteByte('-')
				truocLaGach = true
			}
		}
	}
	// Đuôi ngẫu nhiên: hai doanh nghiệp cùng tên là chuyện thường, và
	// slug trùng làm hồ sơ thứ hai bị từ chối mà người nộp không hiểu vì sao.
	return strings.Trim(b.String(), "-") + "-" +
		strings.ToLower(ids.MustNew(ids.PrefixSeller).String()[26:])
}
