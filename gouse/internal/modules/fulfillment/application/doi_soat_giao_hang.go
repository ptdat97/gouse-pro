package application

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// LostContactParcel là một gói hàng đã bàn giao lâu bất thường mà không có tin.
//
// Mang sẵn mọi thứ người trực cần để đi hỏi đơn vị vận chuyển, để họ không
// phải tra ngược từ id qua bốn bảng lúc 3 giờ sáng.
type LostContactParcel struct {
	FulfillmentID ids.ID
	FONumber      string
	OrderID       ids.ID
	SellerID      ids.ID

	Carrier        string
	TrackingNumber string

	Status    string
	ShippedAt time.Time
	Silent    time.Duration
}

// ReconcileDeliveries tìm các gói hàng MẤT TIN từ đơn vị vận chuyển.
//
// # Đây là nửa NỘI BỘ của yêu cầu 5
//
// `api/paths/webhooks.yaml` yêu cầu "đối chiếu định kỳ, vì webhook có thể
// mất", và đặc tả webhook vận chuyển mô tả "hai cơ chế song song: webhook
// và hỏi định kỳ". Nửa HỎI cần adapter của hãng vận chuyển thật — thứ
// chưa có, cùng lý do với adapter PSP ở ADR-0017.
//
// Nửa làm được ngay là nửa nội bộ: hệ thống TỰ BIẾT gói nào đã im lặng quá
// lâu, mà không cần hỏi ai. Nó không thay thế nửa kia, nhưng nó biến một
// webhook mất từ chỗ vô hình thành một dòng cảnh báo.
//
// # KHÔNG tự đánh dấu đã giao
//
// Hàm này chỉ ĐỌC. Suy ra "chắc là giao rồi" từ việc im lặng là bịa ra một
// sự kiện chưa từng xảy ra — và ở đây nó bịa ra tiền: đánh dấu DELIVERED
// mở đường cho `CompleteDelivered` chuyển số dư nhà bán sang khả dụng, tức
// là trả tiền cho một lần giao hàng không ai xác nhận. Cùng nguyên tắc với
// ADR-0017 phần 4: từ chối chứ không "ghi nhận rồi cảnh báo", vì ghi nhận
// nghĩa là đã tin.
//
// Gói im lặng lâu nhất đứng đầu danh sách; `limit` chặn để một sự cố kéo
// dài không tạo ra hàng nghìn dòng log trong một vòng chạy.
func (s *Service) ReconcileDeliveries(
	ctx context.Context, thresholds time.Duration, limit int,
) ([]LostContactParcel, error) {
	now := s.clock.Now()

	inTransitList, err := s.repo.ListInTransitBefore(ctx, now.Add(-thresholds), limit)
	if err != nil {
		return nil, err
	}

	out := make([]LostContactParcel, 0, len(inTransitList))
	for _, fo := range inTransitList {
		// Hỏi lại domain dù SQL đã lọc: câu truy vấn và quy tắc là hai
		// chỗ, và chỗ quyết định phải là domain. Nếu hai bên lệch nhau
		// thì cái sai lọt ra ngoài là dòng này, không phải cảnh báo.
		if !fo.LostContact(thresholds, now) {
			continue
		}
		out = append(out, LostContactParcel{
			FulfillmentID:  fo.ID(),
			FONumber:       fo.FONumber(),
			OrderID:        fo.OrderID(),
			SellerID:       fo.SellerID(),
			Carrier:        fo.ShippingProvider(),
			TrackingNumber: fo.TrackingNumber(),
			Status:         string(fo.Status()),
			ShippedAt:      fo.ShippedAt(),
			Silent:         fo.SilentFor(now),
		})
	}
	return out, nil
}
