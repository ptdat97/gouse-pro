package application

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// GoiBatTin là một gói hàng đã bàn giao lâu bất thường mà không có tin.
//
// Mang sẵn mọi thứ người trực cần để đi hỏi đơn vị vận chuyển, để họ không
// phải tra ngược từ id qua bốn bảng lúc 3 giờ sáng.
type GoiBatTin struct {
	FulfillmentID ids.ID
	FONumber      string
	OrderID       ids.ID
	SellerID      ids.ID

	NhaVanChuyen string
	MaVanDon     string

	TrangThai string
	ShippedAt time.Time
	ImLang    time.Duration
}

// DoiSoatGiaoHang tìm các gói hàng MẤT TIN từ đơn vị vận chuyển.
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
func (s *Service) DoiSoatGiaoHang(
	ctx context.Context, nguong time.Duration, limit int,
) ([]GoiBatTin, error) {
	now := s.clock.Now()

	dangGiao, err := s.repo.ListDangGiaoTruoc(ctx, now.Add(-nguong), limit)
	if err != nil {
		return nil, err
	}

	out := make([]GoiBatTin, 0, len(dangGiao))
	for _, fo := range dangGiao {
		// Hỏi lại domain dù SQL đã lọc: câu truy vấn và quy tắc là hai
		// chỗ, và chỗ quyết định phải là domain. Nếu hai bên lệch nhau
		// thì cái sai lọt ra ngoài là dòng này, không phải cảnh báo.
		if !fo.BatTinGiaoHang(nguong, now) {
			continue
		}
		out = append(out, GoiBatTin{
			FulfillmentID: fo.ID(),
			FONumber:      fo.FONumber(),
			OrderID:       fo.OrderID(),
			SellerID:      fo.SellerID(),
			NhaVanChuyen:  fo.ShippingProvider(),
			MaVanDon:      fo.TrackingNumber(),
			TrangThai:     string(fo.Status()),
			ShippedAt:     fo.ShippedAt(),
			ImLang:        fo.ImLangBaoLau(now),
		})
	}
	return out, nil
}
