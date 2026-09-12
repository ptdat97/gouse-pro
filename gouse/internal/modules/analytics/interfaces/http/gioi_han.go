package http

import (
	"sync"
	"time"
)

// GioiHanPhien đếm sự kiện theo PHIÊN trong một cửa sổ trượt thô.
//
// # Vì sao giới hạn theo phiên chứ không theo IP
//
// Endpoint này cố ý KHÔNG thu địa chỉ IP (xem chú thích ở handler), nên
// phiên là thứ duy nhất có để phân nhóm. Kẻ lạm dụng đổi `session_id` mỗi
// lần gọi thì lách được — và đó là giới hạn ĐÃ BIẾT của lớp này.
//
// Nó không phải lớp chống tấn công, mà là lớp chặn client HỎNG: một vòng
// lặp gửi nhầm hàng nghìn sự kiện một giây là tình huống thường gặp hơn
// nhiều so với kẻ cố tình phá, và nó cũng làm hỏng dữ liệu y như vậy.
// Chống tấn công có chủ đích là việc của lớp hạ tầng phía trước.
//
// # Cửa sổ THÔ, có chủ ý
//
// Bộ đếm reset trọn khi sang cửa sổ mới, thay vì trượt liên tục. Một client
// vì thế có thể gửi 2N sự kiện quanh ranh giới hai cửa sổ. Chấp nhận được:
// mục tiêu là chặn bậc độ lớn, không phải cưỡng chế chính xác con số — và
// cửa sổ trượt thật cần giữ dấu thời gian từng sự kiện, tức là đổi một
// lượng bộ nhớ lớn lấy độ chính xác không ai cần ở đây.
type GioiHanPhien struct {
	nguong func() int
	cuaSo  time.Duration
	now    func() time.Time

	mu     sync.Mutex
	dem    map[string]int
	batDau time.Time
}

// NewGioiHanPhien tạo bộ giới hạn với ngưỡng đọc LÚC CHẠY.
//
// `nguong` là hàm chứ không phải số: ngưỡng là tham số vận hành, và đổi nó
// phải có tác dụng ở lô kế tiếp chứ không phải ở lần triển khai kế tiếp.
func NewGioiHanPhien(nguong func() int, cuaSo time.Duration) *GioiHanPhien {
	return &GioiHanPhien{
		nguong: nguong,
		cuaSo:  cuaSo,
		now:    func() time.Time { return time.Now() },
		dem:    make(map[string]int),
	}
}

var _ GioiHanPort = (*GioiHanPhien)(nil)

// ChoPhep cộng `soSuKien` vào bộ đếm của phiên và cho biết còn trong ngưỡng.
//
// Ngưỡng <= 0 nghĩa là TẮT giới hạn — dùng khi chưa nối cấu hình.
func (g *GioiHanPhien) ChoPhep(sessionID string, soSuKien int) bool {
	nguong := 0
	if g.nguong != nil {
		nguong = g.nguong()
	}
	if nguong <= 0 {
		return true
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	if now.Sub(g.batDau) >= g.cuaSo {
		// Sang cửa sổ mới: dựng map MỚI thay vì xóa từng khóa.
		//
		// Đây cũng là cách bộ nhớ được thu hồi: không có bước này thì mỗi
		// phiên từng gọi sẽ để lại một khóa vĩnh viễn, và với lưu lượng
		// thật đó là một rò rỉ bộ nhớ chậm nhưng chắc chắn.
		g.dem = make(map[string]int)
		g.batDau = now
	}

	if g.dem[sessionID]+soSuKien > nguong {
		return false
	}
	g.dem[sessionID] += soSuKien
	return true
}
