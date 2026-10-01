package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

// Hai bước KÝ của vòng đời đợt đối soát.
//
// # Vì sao vòng đời này tới 01/10/2026 mới khép
//
// `DoiSoat.XacNhan` và `DoiSoat.DanhDauDaTra` có từ migration 000036 với đủ
// hàng rào trạng thái, và KHÔNG lời gọi nào. Mọi đợt nằm `DRAFT` vĩnh viễn —
// nhà bán không bao giờ được chi.
//
// Khép nó cần một quyết định không thuộc về mã: AI ĐƯỢC KÝ CHI. Quyết 01/10:
// hai bước, có người ký, mỗi bước ghi ai bấm và lúc nào (P3-89).
//
// # Vì sao HAI bước chứ không một
//
//	DRAFT ──XacNhan──► CONFIRMED ──DanhDauDaTra──► PAID
//	       con số thành           tiền RỜI
//	       CAM KẾT                nền tảng
//
// Bước một là lúc một con số TÍNH ĐƯỢC thành một CAM KẾT với nhà bán — số
// đó hiện trên màn hình của họ và họ dựa vào nó. Bước hai là lúc tiền rời
// nền tảng. Gộp lại thì không ai soát con số trước khi nó thành cam kết.
//
// # Vì sao LÝ DO bắt buộc
//
// `audit.WriteSensitive` đòi lý do tối thiểu 20 ký tự. Đó không phải thủ
// tục: với đường tiền, "vì sao" là thứ duy nhất phân biệt một lần chi chính
// đáng với một lần biển thủ — cùng lập luận đã viết cho bút toán điều chỉnh.

// KyDoiSoatInput là dữ liệu ký một bước.
type KyDoiSoatInput struct {
	SettlementID ids.ID

	// NguoiKy là người bấm. BẮT BUỘC.
	NguoiKy ids.ID

	// Reason BẮT BUỘC, tối thiểu 20 ký tự — `audit.WriteSensitive` kiểm.
	Reason    string
	RequestID string
}

// XacNhanDoiSoat chốt đợt đối soát, sẵn sàng chi trả.
//
// IDEMPOTENT theo trạng thái, không theo khóa: gọi lại trên một đợt đã
// `CONFIRMED` trả `ErrInvalidStatus`, và bên gọi HTTP dịch nó thành 409.
// Đó là hành vi đúng cho một thao tác người bấm — khác với event, nơi lặp
// lại là chuyện thường.
func (s *Service) XacNhanDoiSoat(
	ctx context.Context, in KyDoiSoatInput,
) (*domain.DoiSoat, error) {
	return s.kyDoiSoat(ctx, in, "confirm",
		func(d *domain.DoiSoat) error {
			return d.XacNhan(in.NguoiKy, s.clock.Now())
		})
}

// DanhDauDoiSoatDaTra ghi nhận tiền đã chuyển cho nhà bán.
//
// Bước này KHÔNG chuyển tiền — việc chuyển xảy ra ngoài hệ thống (chuyển
// khoản ngân hàng), và endpoint này ghi nhận rằng nó đã xảy ra.
//
// Phân biệt ấy quan trọng: nếu một ngày có cổng chi trả tự động, bước này
// thành bên NHẬN webhook của cổng ấy thay vì một nút người bấm — và hàng rào
// trạng thái cùng vết kiểm toán ở đây không phải viết lại.
func (s *Service) DanhDauDoiSoatDaTra(
	ctx context.Context, in KyDoiSoatInput,
) (*domain.DoiSoat, error) {
	return s.kyDoiSoat(ctx, in, "mark_paid",
		func(d *domain.DoiSoat) error {
			return d.DanhDauDaTra(in.NguoiKy, s.clock.Now())
		})
}

// kyDoiSoat là phần chung của hai bước.
//
// Dùng chung vì hai bước khác nhau ĐÚNG MỘT lời gọi miền; hai bản cài sẽ
// lệch nhau ở phần đọc, kiểm và ghi vết — và phần ấy là phần dễ sai.
func (s *Service) kyDoiSoat(
	ctx context.Context, in KyDoiSoatInput, buoc string,
	doiTrangThai func(*domain.DoiSoat) error,
) (*domain.DoiSoat, error) {
	if s.settlements == nil {
		return nil, errors.New("payment: chưa nối kho đợt đối soát")
	}
	if s.audit == nil {
		// Thất bại theo hướng ĐÓNG: không ghi được vết thì không ký.
		//
		// Khác với các use case ghi sổ tự động, nơi `audit` nil là chấp
		// nhận được — ở đó hành động do HỆ THỐNG thực hiện và event đã là
		// vết. Ở đây có một NGƯỜI bấm, và người ấy là thứ phải ghi lại.
		return nil, errors.New(
			"payment: chưa nối nhật ký kiểm toán — không ký đợt đối soát " +
				"mà không ghi được ai ký")
	}
	if in.NguoiKy.IsZero() {
		return nil, domain.ErrThieuNguoiKy
	}

	d, err := s.settlements.TimTheoID(ctx, in.SettlementID)
	if err != nil {
		return nil, err
	}
	if err := doiTrangThai(d); err != nil {
		return nil, err
	}

	err = s.settlements.LuuKemAudit(ctx, d, func(txCtx context.Context) error {
		return s.audit.RecordSettlementSigned(txCtx, SettlementSignRecord{
			SettlementID: d.ID(),
			SellerID:     d.SellerID(),
			Buoc:         buoc,
			ActorID:      in.NguoiKy.String(),
			Reason:       in.Reason,
			RequestID:    in.RequestID,
			NetAmount:    d.Net().Amount(),
			Currency:     string(d.Net().Currency()),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("payment: ký đợt đối soát %s (%s): %w",
			in.SettlementID, buoc, err)
	}
	return d, nil
}
