package http

import (
	"sync"
	"time"
)

// SessionLimit đếm sự kiện theo PHIÊN trong một cửa sổ trượt thô.
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
type SessionLimit struct {
	threshold func() int
	window    time.Duration
	now       func() time.Time

	mu    sync.Mutex
	count map[string]int
	start time.Time
}

// NewSessionLimit tạo bộ giới hạn với ngưỡng đọc LÚC CHẠY.
//
// `threshold` là hàm chứ không phải số: ngưỡng là tham số vận hành, và đổi nó
// phải có tác dụng ở lô kế tiếp chứ không phải ở lần triển khai kế tiếp.
func NewSessionLimit(threshold func() int, window time.Duration) *SessionLimit {
	return &SessionLimit{
		threshold: threshold,
		window:    window,
		now:       func() time.Time { return time.Now() },
		count:     make(map[string]int),
	}
}

var _ LimitPort = (*SessionLimit)(nil)

// Allowed cộng `eventCount` vào bộ đếm của phiên và cho biết còn trong ngưỡng.
//
// Ngưỡng <= 0 nghĩa là TẮT giới hạn — dùng khi chưa nối cấu hình.
func (g *SessionLimit) Allowed(sessionID string, eventCount int) bool {
	threshold := 0
	if g.threshold != nil {
		threshold = g.threshold()
	}
	if threshold <= 0 {
		return true
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	if now.Sub(g.start) >= g.window {
		// Sang cửa sổ mới: dựng map MỚI thay vì xóa từng khóa.
		//
		// Đây cũng là cách bộ nhớ được thu hồi: không có bước này thì mỗi
		// phiên từng gọi sẽ để lại một khóa vĩnh viễn, và với lưu lượng
		// thật đó là một rò rỉ bộ nhớ chậm nhưng chắc chắn.
		g.count = make(map[string]int)
		g.start = now
	}

	if g.count[sessionID]+eventCount > threshold {
		return false
	}
	g.count[sessionID] += eventCount
	return true
}
