package app

import (
	"context"

	fulfillmenthttp "github.com/fashion-commerce/platform/internal/modules/fulfillment/interfaces/http"
	paymenthttp "github.com/fashion-commerce/platform/internal/modules/payment/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/webhook"
)

// webhookLog nối cổng GhiSuKien của module fulfillment với kho nhật ký
// webhook ở platform.
//
// Adapter tồn tại vì module KHÔNG được import platform/webhook trực tiếp
// — nó sẽ biến một chi tiết hạ tầng thành phụ thuộc của tầng nghiệp vụ.
// Đổi lại là mười dòng chuyển kiểu ở đây, tại điểm khởi chạy.
type webhookLog struct{ r *webhook.Recorder }

var _ fulfillmenthttp.WebhookLog = (*webhookLog)(nil)

func (g *webhookLog) Log(
	ctx context.Context, provider, eventID, eventKind string, than []byte,
) (fulfillmenthttp.LoggedEvent, error) {
	logEntry, err := g.r.Log(ctx, provider, eventID, eventKind, than)
	return fulfillmenthttp.LoggedEvent{
		ID:              logEntry.ID,
		AlreadyReceived: logEntry.AlreadyReceived,
		AlreadyDone:     logEntry.AlreadyDone,
	}, err
}

func (g *webhookLog) MarkDone(ctx context.Context, id string, errs error) error {
	return g.r.MarkDone(ctx, id, errs)
}

// webhookSecrets tra khóa HMAC theo mã nhà cung cấp.
//
// Nhà cung cấp không có trong cấu hình trả chuỗi RỖNG, và
// httpserver.VerifyHMAC coi khóa rỗng là chữ ký không hợp lệ. Mặc định
// là ĐÓNG: hãng chưa cấu hình thì không gửi được gì vào hệ thống.
func webhookSecrets(table map[string]string) fulfillmenthttp.ProviderSecret {
	return func(provider string) string { return table[provider] }
}

// paymentWebhookLog là CÙNG kho nhật ký, nhìn qua cổng của module payment.
//
// Hai module khai hai interface giống hệt nhau và đó là chủ ý: dùng chung
// một kiểu nghĩa là một trong hai module phải import module kia, hoặc cả
// hai phải import platform/webhook. Mười dòng chuyển kiểu ở điểm khởi chạy
// rẻ hơn cả hai lựa chọn đó.
type paymentWebhookLog struct{ r *webhook.Recorder }

var _ paymenthttp.WebhookLog = (*paymentWebhookLog)(nil)

func (g *paymentWebhookLog) Log(
	ctx context.Context, provider, eventID, eventKind string, than []byte,
) (paymenthttp.LoggedEvent, error) {
	logEntry, err := g.r.Log(ctx, provider, eventID, eventKind, than)
	return paymenthttp.LoggedEvent{
		ID:              logEntry.ID,
		AlreadyReceived: logEntry.AlreadyReceived,
		AlreadyDone:     logEntry.AlreadyDone,
	}, err
}

func (g *paymentWebhookLog) MarkDone(ctx context.Context, id string, errs error) error {
	return g.r.MarkDone(ctx, id, errs)
}

// paymentWebhookSecret dùng CHUNG bảng khóa với webhook vận chuyển.
//
// Khóa tra theo mã nhà cung cấp, và một hãng vận chuyển không trùng tên
// với một cổng thanh toán. Mặc định vẫn ĐÓNG: hãng chưa cấu hình thì chữ
// ký không bao giờ hợp lệ.
func paymentWebhookSecret(table map[string]string) paymenthttp.ProviderSecret {
	return func(provider string) string { return table[provider] }
}
