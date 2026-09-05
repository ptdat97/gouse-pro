package domain

import (
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// MocPhanTrang là vị trí đọc tiếp trong lịch sử đơn, theo KHÓA của bản ghi
// cuối trang trước — không phải số bản ghi cần bỏ qua.
//
// # Vì sao không dùng offset
//
// Offset đếm theo VỊ TRÍ trong một danh sách đang thay đổi. Danh sách đơn
// sắp theo `placed_at DESC`, nên một đơn mới chèn vào ĐẦU và đẩy mọi bản
// ghi lùi một bậc. Trang sau đọc lại đúng bản ghi trang trước đã trả:
// khách thấy cùng một đơn hai lần.
//
// Chiều ngược lại tệ hơn vì im lặng: bản ghi rời khỏi tập lọc giữa hai lần
// đọc thì mọi thứ dồn lên, một đơn bị NHẢY QUA và không bao giờ hiện ra.
//
// Khóa thì không đếm vị trí — nó hỏi "đơn nào cũ hơn đơn này" — nên chèn
// hay xóa ở phần đã đọc không ảnh hưởng gì tới phần chưa đọc.
//
// # Vì sao cần CẢ hai trường
//
// `placed_at` không phải khóa duy nhất: hai đơn đặt trong cùng một
// micro-giây có chung mốc. Nếu ranh giới trang rơi đúng vào chỗ trùng đó
// thì so sánh theo mình `placed_at` hoặc bỏ sót hoặc lặp các đơn cùng mốc.
// Cặp `(placed_at, id)` là duy nhất vì `id` là khóa chính.
type MocPhanTrang struct {
	PlacedAt time.Time
	ID       ids.ID
}
