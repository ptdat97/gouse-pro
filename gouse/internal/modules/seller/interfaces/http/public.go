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
