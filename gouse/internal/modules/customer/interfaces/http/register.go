package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// RegisterPort là những gì tầng HTTP cần để tạo tài khoản khách hàng.
//
// Interface do BÊN GỌI khai báo: tầng interfaces không được import gói cha
// (`customer`) vì gói cha đã import nó — vòng lặp. Module tự nối vào.
type RegisterPort interface {
	RegisterShopper(ctx context.Context, req RegisterInput) (RegisterOutput, error)
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
}

func NewRegisterHandler(
	port RegisterPort, log *slog.Logger,
	duplicateEmail, emailUsedByGuest, weakPassword error,
) *RegisterHandler {
	return &RegisterHandler{
		port: port, log: log,
		duplicateEmail:   duplicateEmail,
		emailUsedByGuest: emailUsedByGuest,
		weakPassword:     weakPassword,
	}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc httpserver.RateLimit: endpoint này trả lời
// được câu "email này có tài khoản chưa", nên không giới hạn tần suất thì
// nó là công cụ dò danh sách email (identity/public.go ghi rõ ràng buộc).
func (h *RegisterHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/register", http.HandlerFunc(h.register))
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

	h.ok(w, r, http.StatusCreated, registerResponse{
		CustomerID: out.CustomerID,
		UserID:     out.UserID,
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
