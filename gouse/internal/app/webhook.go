package app

import (
	"context"

	fulfillmenthttp "github.com/fashion-commerce/platform/internal/modules/fulfillment/interfaces/http"
	paymenthttp "github.com/fashion-commerce/platform/internal/modules/payment/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/webhook"
)

// ghiSuKien nối cổng GhiSuKien của module fulfillment với kho nhật ký
// webhook ở platform.
//
// Adapter tồn tại vì module KHÔNG được import platform/webhook trực tiếp
// — nó sẽ biến một chi tiết hạ tầng thành phụ thuộc của tầng nghiệp vụ.
// Đổi lại là mười dòng chuyển kiểu ở đây, tại điểm khởi chạy.
type ghiSuKien struct{ r *webhook.Recorder }

var _ fulfillmenthttp.WebhookLog = (*ghiSuKien)(nil)

func (g *ghiSuKien) Log(
	ctx context.Context, nhaCungCap, maSuKien, loaiSuKien string, than []byte,
) (fulfillmenthttp.LoggedEvent, error) {
	su, err := g.r.Log(ctx, nhaCungCap, maSuKien, loaiSuKien, than)
	return fulfillmenthttp.LoggedEvent{
		ID:              su.ID,
		AlreadyReceived: su.AlreadyReceived,
		AlreadyDone:     su.AlreadyDone,
	}, err
}

func (g *ghiSuKien) MarkDone(ctx context.Context, id string, loi error) error {
	return g.r.MarkDone(ctx, id, loi)
}

// biMatWebhook tra khóa HMAC theo mã nhà cung cấp.
//
// Nhà cung cấp không có trong cấu hình trả chuỗi RỖNG, và
// httpserver.VerifyHMAC coi khóa rỗng là chữ ký không hợp lệ. Mặc định
// là ĐÓNG: hãng chưa cấu hình thì không gửi được gì vào hệ thống.
func biMatWebhook(bang map[string]string) fulfillmenthttp.ProviderSecret {
	return func(nhaCungCap string) string { return bang[nhaCungCap] }
}

// ghiSuKienThanhToan là CÙNG kho nhật ký, nhìn qua cổng của module payment.
//
// Hai module khai hai interface giống hệt nhau và đó là chủ ý: dùng chung
// một kiểu nghĩa là một trong hai module phải import module kia, hoặc cả
// hai phải import platform/webhook. Mười dòng chuyển kiểu ở điểm khởi chạy
// rẻ hơn cả hai lựa chọn đó.
type ghiSuKienThanhToan struct{ r *webhook.Recorder }

var _ paymenthttp.WebhookLog = (*ghiSuKienThanhToan)(nil)

func (g *ghiSuKienThanhToan) Log(
	ctx context.Context, nhaCungCap, maSuKien, loaiSuKien string, than []byte,
) (paymenthttp.LoggedEvent, error) {
	su, err := g.r.Log(ctx, nhaCungCap, maSuKien, loaiSuKien, than)
	return paymenthttp.LoggedEvent{
		ID:              su.ID,
		AlreadyReceived: su.AlreadyReceived,
		AlreadyDone:     su.AlreadyDone,
	}, err
}

func (g *ghiSuKienThanhToan) MarkDone(ctx context.Context, id string, loi error) error {
	return g.r.MarkDone(ctx, id, loi)
}

// biMatWebhookThanhToan dùng CHUNG bảng khóa với webhook vận chuyển.
//
// Khóa tra theo mã nhà cung cấp, và một hãng vận chuyển không trùng tên
// với một cổng thanh toán. Mặc định vẫn ĐÓNG: hãng chưa cấu hình thì chữ
// ký không bao giờ hợp lệ.
func biMatWebhookThanhToan(bang map[string]string) paymenthttp.ProviderSecret {
	return func(nhaCungCap string) string { return bang[nhaCungCap] }
}
