package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// RegisterPort là những gì tầng HTTP cần để tạo tài khoản khách hàng.
//
// Interface do BÊN GỌI khai báo: tầng interfaces không được import gói cha
// (`customer`) vì gói cha đã import nó — vòng lặp. Module tự nối vào.
type RegisterPort interface {
	RegisterShopper(ctx context.Context, req RegisterInput) (RegisterOutput, error)

	// XacMinhEmailVaGop xác minh email rồi gộp hồ sơ vãng lai (P3-15).
	XacMinhEmailVaGop(ctx context.Context, token string) (XacMinhOutput, error)

	// GuiLienKetXacMinh phát token mới và gửi thư chứa liên kết.
	GuiLienKetXacMinh(ctx context.Context, userID string) error
}

// XacMinhOutput là kết quả xác minh email.
type XacMinhOutput struct {
	Email      string
	DaGopHoSo  bool
	SoDonDaGop int
}

// RegisterInput là dữ liệu đăng ký.
type RegisterInput struct {
	Email       string
	Password    string
	Phone       string
	DisplayName string
}

// RegisterOutput là kết quả đăng ký.
type RegisterOutput struct {
	CustomerID string
	UserID     string

	// CanXacMinhEmail = true khi email đã có hồ sơ vãng lai; lịch sử mua
	// hàng chỉ được gộp sau khi xác minh.
	CanXacMinhEmail bool
}

// RegisterHandler phục vụ đường ĐĂNG KÝ.
//
// Tách khỏi Handler vì nó là endpoint CÔNG KHAI — mọi endpoint kia yêu cầu
// đã đăng nhập. Gộp chung thì sớm muộn có người thêm đường công khai vào
// nhóm cần đăng nhập, hoặc ngược lại.
type RegisterHandler struct {
	port RegisterPort
	log  *slog.Logger

	// duplicateEmail và emailUsedByGuest là hai lỗi cần phân biệt cho
	// người dùng, do gói cha định nghĩa và truyền vào.
	duplicateEmail   error
	emailUsedByGuest error
	weakPassword     error

	// tokenKhongHopLe và emailDaDoi là lỗi của luồng XÁC MINH email.
	tokenKhongHopLe error
	emailDaDoi      error
}

// RegisterHandlerErrors gom các lỗi mà gói cha định nghĩa.
//
// Gom thành struct thay vì thêm tham số thứ sáu, thứ bảy: một hàm dựng
// nhận sáu `error` liền nhau là chỗ để hoán vị hai đối số mà trình biên
// dịch không thấy — và khi đó thông báo lỗi hiện ra cho người dùng là
// thông báo của tình huống khác.
type RegisterHandlerErrors struct {
	DuplicateEmail   error
	EmailUsedByGuest error
	WeakPassword     error
	TokenKhongHopLe  error
	EmailDaDoi       error
}

func NewRegisterHandler(
	port RegisterPort, log *slog.Logger, errs RegisterHandlerErrors,
) *RegisterHandler {
	return &RegisterHandler{
		port: port, log: log,
		duplicateEmail:   errs.DuplicateEmail,
		emailUsedByGuest: errs.EmailUsedByGuest,
		weakPassword:     errs.WeakPassword,
		tokenKhongHopLe:  errs.TokenKhongHopLe,
		emailDaDoi:       errs.EmailDaDoi,
	}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc httpserver.RateLimit: endpoint này trả lời
// được câu "email này có tài khoản chưa", nên không giới hạn tần suất thì
// nó là công cụ dò danh sách email (identity/public.go ghi rõ ràng buộc).
func (h *RegisterHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/register", http.HandlerFunc(h.register))

	// Xác minh email cũng CẦN giới hạn tần suất: nó nhận token từ bên
	// ngoài, và không giới hạn thì nó là chỗ để dò token bằng vét cạn.
	mux.Handle("POST /api/v1/auth/verify-email", http.HandlerFunc(h.xacMinh))
}

type xacMinhRequest struct {
	Token string `json:"token"`
}

type xacMinhResponse struct {
	Email string `json:"email"`

	// ProfileMerged cho giao diện biết hồ sơ vãng lai vừa được gắn vào
	// tài khoản.
	ProfileMerged bool `json:"profile_merged"`

	// OrdersMerged là SỐ ĐƠN cũ vừa về với khách — thứ họ thật sự chờ.
	//
	// Nói ra con số chứ không chỉ true/false: "đã tìm thấy 3 đơn cũ" là
	// câu khách kiểm chứng được ngay, còn "đã gộp" thì không.
	OrdersMerged int `json:"orders_merged"`
}

// xacMinh phục vụ POST /api/v1/auth/verify-email (operationId: verifyEmail).
//
// # Vì sao KHÔNG cần đăng nhập
//
// Người bấm liên kết trong hộp thư thường đang ở trình duyệt khác, hoặc
// trên điện thoại. Bắt đăng nhập trước là đẩy họ qua một bước mà chính
// bước xác minh này tồn tại để giúp họ vượt qua.
//
// Token đã là bằng chứng: nó ngẫu nhiên đủ mạnh, dùng MỘT lần, và hết hạn
// sau 24 giờ.
func (h *RegisterHandler) xacMinh(w http.ResponseWriter, r *http.Request) {
	var req xacMinhRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"token là trường bắt buộc"))
		return
	}

	out, err := h.port.XacMinhEmailVaGop(r.Context(), strings.TrimSpace(req.Token))
	if err != nil {
		h.fail(w, r, h.translateXacMinh(err))
		return
	}

	h.ok(w, r, http.StatusOK, xacMinhResponse{
		Email:         out.Email,
		ProfileMerged: out.DaGopHoSo,
		OrdersMerged:  out.SoDonDaGop,
	})
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`

	Phone string `json:"phone,omitempty"`
	Name  string `json:"name,omitempty"`
}

type registerResponse struct {
	CustomerID string `json:"customer_id"`
	UserID     string `json:"user_id"`

	// EmailVerificationRequired = true khi email này đã có lịch sử mua
	// hàng vãng lai. Tài khoản đã tạo và dùng được, nhưng lịch sử cũ chỉ
	// hiện ra sau khi khách bấm liên kết xác minh trong hộp thư.
	EmailVerificationRequired bool `json:"email_verification_required,omitempty"`
}

// register phục vụ POST /api/v1/auth/register (operationId: registerCustomer).
//
// # KHÔNG trả token — client phải gọi `login` ngay sau đó
//
// Phát hành token là việc của module identity (nó giữ bộ ký và quản lý
// phiên). Làm ở đây nghĩa là nhân bản logic phiên đăng nhập ra chỗ thứ hai.
// Một lượt gọi thêm là cái giá rẻ hơn nhiều.
func (h *RegisterHandler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.Email == "" || req.Password == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"email và password là trường bắt buộc"))
		return
	}

	out, err := h.port.RegisterShopper(r.Context(), RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		Phone:       req.Phone,
		DisplayName: req.Name,
	})
	if err != nil {
		h.fail(w, r, h.translateRegister(err))
		return
	}

	// Gửi thư xác minh khi email này đã có lịch sử vãng lai.
	//
	// Gửi hỏng KHÔNG làm hỏng việc đăng ký: tài khoản đã tạo và dùng được,
	// và khách bấm "gửi lại" là xong. Trả lỗi ở đây sẽ khiến họ đăng ký
	// lại rồi nhận "email đã có tài khoản" — của chính mình.
	//
	// Nhưng cũng KHÔNG nuốt im lặng: ghi ERROR để người vận hành thấy khi
	// đường gửi thư hỏng hàng loạt.
	if out.CanXacMinhEmail {
		if err := h.port.GuiLienKetXacMinh(r.Context(), out.UserID); err != nil {
			h.log.ErrorContext(r.Context(),
				"đăng ký xong nhưng KHÔNG gửi được thư xác minh",
				"error", err, "user_id", out.UserID,
				"goi_y", "khách bấm gửi lại được; kiểm tra đường gửi thư")
		}
	}

	h.ok(w, r, http.StatusCreated, registerResponse{
		CustomerID:                out.CustomerID,
		UserID:                    out.UserID,
		EmailVerificationRequired: out.CanXacMinhEmail,
	})
}

// translateRegister phân biệt hai lý do "email đã dùng".
//
// Chúng dẫn tới hai hành động KHÁC HẲN của người dùng:
//
//	đã có TÀI KHOẢN   → đăng nhập, hoặc quên mật khẩu
//	đã ĐẶT HÀNG vãng lai → tra đơn bằng mã + số điện thoại
//
// Trả chung một thông báo "email đã dùng" đẩy nhóm thứ hai vào đường cụt:
// họ bấm "quên mật khẩu" cho một tài khoản không tồn tại.
func (h *RegisterHandler) translateRegister(err error) error {
	switch {
	case h.emailUsedByGuest != nil && errors.Is(err, h.emailUsedByGuest):
		return apierror.New(apierror.CodeConflict,
			"Email này đã từng được dùng để đặt hàng. Bạn tra cứu đơn cũ "+
				"bằng mã đơn và số điện thoại — không cần tài khoản.")

	case h.duplicateEmail != nil && errors.Is(err, h.duplicateEmail):
		return apierror.New(apierror.CodeConflict,
			"Email này đã có tài khoản. Vui lòng đăng nhập.")

	case h.weakPassword != nil && errors.Is(err, h.weakPassword):
		return apierror.New(apierror.CodeValidationFailed,
			"Mật khẩu quá ngắn")

	default:
		return apierror.From(err)
	}
}

// translateXacMinh dịch lỗi xác minh email thành thông báo hữu ích.
//
// Token hỏng vì BA lý do khác nhau — không tồn tại, đã dùng, đã hết hạn —
// và cả ba trả CÙNG một thông báo, có chủ ý: phân biệt chúng cho kẻ dò
// biết token nào TỪNG có thật.
//
// Thông báo vẫn nêu HÀNH ĐỘNG cần làm ("yêu cầu gửi lại"), vì người dùng
// thật gặp lỗi này chủ yếu do liên kết cũ quá 24 giờ.
func (h *RegisterHandler) translateXacMinh(err error) error {
	switch {
	case h.tokenKhongHopLe != nil && errors.Is(err, h.tokenKhongHopLe),
		h.emailDaDoi != nil && errors.Is(err, h.emailDaDoi):
		return apierror.New(apierror.CodeValidationFailed,
			"Liên kết xác minh không còn hiệu lực. Hãy yêu cầu gửi lại.")
	default:
		return apierror.From(err)
	}
}

func (h *RegisterHandler) ok(
	w http.ResponseWriter, r *http.Request, status int, body any,
) {
	if err := apierror.WriteJSON(w, status, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *RegisterHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}
