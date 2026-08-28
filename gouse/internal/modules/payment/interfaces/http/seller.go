package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// SellerHandler phục vụ endpoint số dư của NHÀ BÁN.
//
// Tách khỏi Handler quản trị vì hai bên thấy hai thứ khác nhau: nhà bán
// chỉ thấy số dư CỦA MÌNH, còn quản trị thấy sổ cái toàn nền tảng.
type SellerHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewSellerHandler(svc *application.Service, log *slog.Logger) *SellerHandler {
	return &SellerHandler{svc: svc, log: log}
}

func (h *SellerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/seller/balance", http.HandlerFunc(h.soDu))
	mux.Handle("GET /api/v1/seller/settlements", http.HandlerFunc(h.danhSachDoiSoat))
	mux.Handle("GET /api/v1/seller/settlements/{settlement_id}",
		http.HandlerFunc(h.motDoiSoat))
}

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// balanceJSON khớp schema `SellerBalance` trong đặc tả.
//
// Năm trạng thái, nhưng chỉ hai cái CÓ THẬT trong sổ cái hôm nay. Ba cái
// còn lại trả 0 — và 0 là con số ĐÚNG: chưa có luồng chi trả nên không
// đồng nào "đang xử lý", chưa có cơ chế giữ tiền nên không đồng nào bị
// giữ, chưa có chính sách reserve nên không đồng nào bị giữ lại.
//
// Giữ đủ năm trường thay vì bỏ bớt: bên gọi đọc theo đặc tả, và một trường
// biến mất là một chỗ họ phải viết mã phòng thủ.
type balanceJSON struct {
	Currency string `json:"currency"`

	Pending   moneyJSON `json:"pending"`
	Available moneyJSON `json:"available"`

	// Ba trạng thái dưới đây CHƯA được mô hình hóa — xem
	// application.GetSoDuNhaBan.
	Processing  moneyJSON `json:"processing"`
	OnHold      moneyJSON `json:"on_hold"`
	ReserveHeld moneyJSON `json:"reserve_held"`
}

func (h *SellerHandler) soDu(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	sd, err := h.svc.GetSoDuNhaBan(r.Context(), sellerID)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	tienTe := string(money.VND)
	if c := sd.RutDuoc.Currency(); c != "" {
		tienTe = string(c)
	} else if c := sd.DangCho.Currency(); c != "" {
		tienTe = string(c)
	}

	khong := moneyJSON{Amount: 0, Currency: tienTe}
	body := balanceJSON{
		Currency:    tienTe,
		Pending:     moneyJSON{Amount: sd.DangCho.Amount(), Currency: tienTe},
		Available:   moneyJSON{Amount: sd.RutDuoc.Amount(), Currency: tienTe},
		Processing:  khong,
		OnHold:      khong,
		ReserveHeld: khong,
	}

	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}

func (h *SellerHandler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"seller balance chạy không qua Auth — kiểm tra nối dây")
		return "", apierror.ErrUnauthorized
	}
	if len(ac.SellerIDs) == 0 {
		return "", apierror.New(apierror.CodeForbidden,
			"Tài khoản này không gắn với nhà bán nào")
	}
	return ids.ID(strings.TrimSpace(ac.SellerIDs[0])), nil
}

func (h *SellerHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// ---------------------------------------------------------------- Đối soát

type settlementJSON struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Currency string `json:"currency"`

	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`

	// Gross là tổng khoản rút được gom trong đợt.
	Gross moneyJSON `json:"gross_amount"`

	// Deficit là phần nhà bán đang NỢ, trừ ra khỏi số thực chi.
	//
	// Hiện ra ngoài chứ không giấu: nhà bán thấy số thực chi nhỏ hơn tổng
	// và cần biết vì sao, nếu không mọi đợt như thế đều thành khiếu nại.
	Deficit moneyJSON `json:"deficit_amount"`

	// Net là số ĐEM ĐI CHI TRẢ.
	Net moneyJSON `json:"net_amount"`

	CreatedAt string `json:"created_at"`
}

func toSettlementJSON(d *domain.DoiSoat) settlementJSON {
	m := func(x money.Money) moneyJSON {
		return moneyJSON{Amount: x.Amount(), Currency: string(x.Currency())}
	}
	out := settlementJSON{
		ID: d.ID().String(), Status: string(d.Status()),
		Currency:    string(d.Net().Currency()),
		PeriodStart: d.PeriodStart().Format(time.RFC3339),
		PeriodEnd:   d.PeriodEnd().Format(time.RFC3339),
		Gross:       m(d.Gross()), Deficit: m(d.Deficit()), Net: m(d.Net()),
		CreatedAt: d.CreatedAt().Format(time.RFC3339),
	}
	return out
}

func (h *SellerHandler) danhSachDoiSoat(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ds, err := h.svc.DanhSachDoiSoat(r.Context(), sellerID, 50)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]settlementJSON, 0, len(ds))
	for _, d := range ds {
		out = append(out, toSettlementJSON(d))
	}
	if err := apierror.WriteJSON(w, http.StatusOK,
		map[string]any{"data": out}); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}

func (h *SellerHandler) motDoiSoat(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	d, err := h.svc.LayDoiSoat(r.Context(),
		ids.ID(r.PathValue("settlement_id")), sellerID)
	if err != nil {
		if errors.Is(err, domain.ErrSettlementNotFound) {
			h.fail(w, r, apierror.New(apierror.CodeNotFound,
				"Không tìm thấy đợt đối soát"))
			return
		}
		h.fail(w, r, err)
		return
	}

	if err := apierror.WriteJSON(w, http.StatusOK,
		map[string]any{"settlement": toSettlementJSON(d)}); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}
