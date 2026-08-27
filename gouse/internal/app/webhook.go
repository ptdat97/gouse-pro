package app

import (
	"context"

	fulfillmenthttp "github.com/fashion-commerce/platform/internal/modules/fulfillment/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/webhook"
)

// ghiSuKien nối cổng GhiSuKien của module fulfillment với kho nhật ký
// webhook ở platform.
//
// Adapter tồn tại vì module KHÔNG được import platform/webhook trực tiếp
// — nó sẽ biến một chi tiết hạ tầng thành phụ thuộc của tầng nghiệp vụ.
// Đổi lại là mười dòng chuyển kiểu ở đây, tại điểm khởi chạy.
type ghiSuKien struct{ r *webhook.Recorder }

var _ fulfillmenthttp.GhiSuKien = (*ghiSuKien)(nil)

func (g *ghiSuKien) Ghi(
	ctx context.Context, nhaCungCap, maSuKien, loaiSuKien string, than []byte,
) (fulfillmenthttp.SuKienDaGhi, error) {
	su, err := g.r.Ghi(ctx, nhaCungCap, maSuKien, loaiSuKien, than)
	return fulfillmenthttp.SuKienDaGhi{
		ID:            su.ID,
		DaNhanTruocDo: su.DaNhanTruocDo,
		DaXuLyXong:    su.DaXuLyXong,
	}, err
}

func (g *ghiSuKien) DanhDauXong(ctx context.Context, id string, loi error) error {
	return g.r.DanhDauXong(ctx, id, loi)
}

// biMatWebhook tra khóa HMAC theo mã nhà cung cấp.
//
// Nhà cung cấp không có trong cấu hình trả chuỗi RỖNG, và
// httpserver.KiemChuKyHMAC coi khóa rỗng là chữ ký không hợp lệ. Mặc định
// là ĐÓNG: hãng chưa cấu hình thì không gửi được gì vào hệ thống.
func biMatWebhook(bang map[string]string) fulfillmenthttp.BiMatNhaCungCap {
	return func(nhaCungCap string) string { return bang[nhaCungCap] }
}
