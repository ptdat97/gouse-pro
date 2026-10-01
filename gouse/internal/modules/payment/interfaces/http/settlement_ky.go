package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Hai endpoint KÝ đợt đối soát — mắt xích cuối của đường tiền ra.
//
// Tới 01/10/2026 `DoiSoat.XacNhan` và `DanhDauDaTra` không có lời gọi nào,
// nên mọi đợt nằm `DRAFT` vĩnh viễn và nhà bán không bao giờ được chi
// (P3-89).
//
// Cùng bộ lớp bảo vệ với điều chỉnh sổ cái, vì cùng mức rủi ro:
//
//	Hàng rào trạng thái  — domain từ chối bước sai thứ tự
//	Người ký BẮT BUỘC    — domain + CHECK ở tầng database
//	Lý do ≥ 20 ký tự     — audit.WriteSensitive từ chối lý do rác
//	Idempotency-Key      — middleware chặn thử lại vì mạng chập chờn
//	Vết kiểm toán        — CÙNG giao dịch với trạng thái đợt
//	Vai trò ADMIN/OPS_FINANCE + 2FA — nối ở tầng route

// kySettlementRequest là thân của cả hai endpoint.
//
// CHỈ có `reason`: người ký lấy từ token, không nhận từ thân request. Nhận
// từ thân nghĩa là ai gọi được endpoint cũng ghi được tên người khác vào vết
// kiểm toán — và vết ấy là thứ duy nhất nói ai chịu trách nhiệm.
type kySettlementRequest struct {
	Reason string `json:"reason"`
}

// settlementResponse khớp components/schemas.yaml#/Settlement.
type settlementResponse struct {
	Settlement settlementJSON `json:"settlement"`
}

// themNguoiKy điền hai trường NGƯỜI KÝ cho mặt quản trị.
//
// `toSettlementJSON` (seller.go) cố ý để trống chúng: nhà bán không cần biết
// nhân viên nào bấm nút. Một hàm bọc thay vì một tham số cờ, vì cờ sẽ lan
// vào mọi chỗ gọi và sớm muộn có chỗ truyền sai.
func themNguoiKy(j settlementJSON, d *domain.DoiSoat) settlementJSON {
	j.ConfirmedBy = d.ConfirmedBy().String()
	j.PaidBy = d.PaidBy().String()
	return j
}

// listSettlements liệt kê đợt của MỌI nhà bán — mặt QUẢN TRỊ.
//
// Không có endpoint này thì hai nút ký ở trên không có đường nào tới: màn
// hình vận hành cần danh sách trước khi cần nút. Một endpoint ghi mà không
// có endpoint đọc là một nút không ai bấm được.
func (h *Handler) listSettlements(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.DanhSachDoiSoatQuanTri(r.Context(),
		r.URL.Query().Get("status"), docLimit(r))
	if err != nil {
		h.fail(w, r, translateDoiSoat(err))
		return
	}

	out := make([]settlementJSON, 0, len(ds))
	for _, d := range ds {
		out = append(out, themNguoiKy(toSettlementJSON(d), d))
	}
	h.ok(w, r, map[string]any{"data": out})
}

// docLimit đọc `limit`, kẹp về khoảng hợp lệ.
//
// Giá trị lạ rơi về mặc định thay vì báo lỗi: đây là tham số TRÌNH BÀY, và
// một màn hình vận hành không nên trắng vì ai đó gõ `limit=abc`.
func docLimit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 100 {
		return 50
	}
	return n
}

func (h *Handler) confirmSettlement(w http.ResponseWriter, r *http.Request) {
	h.kySettlement(w, r,
		func(r *http.Request, in application.KyDoiSoatInput) (*domain.DoiSoat, error) {
			return h.svc.XacNhanDoiSoat(r.Context(), in)
		})
}

func (h *Handler) markSettlementPaid(w http.ResponseWriter, r *http.Request) {
	h.kySettlement(w, r,
		func(r *http.Request, in application.KyDoiSoatInput) (*domain.DoiSoat, error) {
			return h.svc.DanhDauDoiSoatDaTra(r.Context(), in)
		})
}

// kySettlement là phần chung của hai bước.
//
// Hai endpoint khác nhau ĐÚNG MỘT lời gọi; hai bản cài sẽ lệch nhau ở phần
// đọc token, kiểm khóa idempotency và dịch lỗi — và phần ấy là phần dễ sai.
func (h *Handler) kySettlement(
	w http.ResponseWriter, r *http.Request,
	ky func(*http.Request, application.KyDoiSoatInput) (*domain.DoiSoat, error),
) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"ký đợt đối soát chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	if _, ok := httpserver.IdempotencyKeyFrom(r.Context()); !ok {
		// Thất bại theo hướng an toàn, cùng lý do với điều chỉnh sổ cái:
		// không có khóa thì một lần thử lại vì mạng chập chờn sẽ ghi vết
		// kiểm toán thứ hai cho cùng một lần ký.
		h.log.ErrorContext(r.Context(),
			"ký đợt đối soát chạy không qua RequireIdempotencyKey")
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Thiếu header Idempotency-Key"))
		return
	}

	id := r.PathValue("settlement_id")
	if _, err := ids.Parse(id, ids.PrefixSettlement); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeNotFound,
			"Không tìm thấy đợt đối soát"))
		return
	}

	var req kySettlementRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	d, err := ky(r, application.KyDoiSoatInput{
		SettlementID: ids.ID(id),
		NguoiKy:      ids.ID(ac.UserID),
		Reason:       req.Reason,
		RequestID:    logger.RequestIDFromContext(r.Context()),
	})
	if err != nil {
		h.fail(w, r, translateDoiSoat(err))
		return
	}

	h.ok(w, r, settlementResponse{
		Settlement: themNguoiKy(toSettlementJSON(d), d),
	})
}

// translateDoiSoat dịch lỗi miền của đợt đối soát sang lỗi API.
//
// `ErrInvalidStatus` thành 409 chứ không 400: bên gọi gửi một request HỢP LỆ,
// chỉ là đợt đã ở trạng thái khác — thường vì người khác vừa bấm. 400 sẽ
// khiến giao diện bảo người dùng sửa dữ liệu, mà không có gì để sửa.
func translateDoiSoat(err error) error {
	switch {
	case errors.Is(err, domain.ErrSettlementNotFound):
		return apierror.New(apierror.CodeNotFound,
			"Không tìm thấy đợt đối soát")

	case errors.Is(err, domain.ErrInvalidStatus):
		return apierror.New(apierror.CodeConflict,
			"Đợt đối soát không ở trạng thái cho phép bước này")

	case errors.Is(err, domain.ErrVersionConflict):
		return apierror.New(apierror.CodeConflict,
			"Đợt đối soát vừa được người khác thay đổi — tải lại rồi thử lại")

	case errors.Is(err, domain.ErrSettlementNotPayable):
		return apierror.New(apierror.CodeValidationFailed,
			"Đợt đối soát không có số dương để chi trả")

	case errors.Is(err, domain.ErrThieuNguoiKy):
		return apierror.New(apierror.CodeUnauthorized,
			"Không xác định được người ký")

	default:
		return translate(err)
	}
}
