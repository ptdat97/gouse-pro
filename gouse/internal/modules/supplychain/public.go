// Package supplychain ghi nhận tín hiệu nhu cầu và (từ Phase 3) lập kế
// hoạch sản xuất.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// # Phạm vi MVP: CHỈ GHI TÍN HIỆU
//
// Chưa có dự báo, chưa có lập kế hoạch, chưa có giao diện. Nhưng module
// phải tồn tại từ MVP vì một lý do duy nhất:
//
//	DỮ LIỆU LỊCH SỬ KHÔNG TẠO NGƯỢC ĐƯỢC.
//
// Tới Phase 3 mà thiếu dữ liệu hành vi của 12 tháng trước thì không dự báo
// được gì, và không có cách nào dựng lại nó.
//
// # Ba tín hiệu giá trị nhất
//
// SEARCH_NO_RESULT, STOCKOUT và NOTIFY_REQUEST đo NHU CẦU KHÔNG ĐƯỢC ĐÁP
// ỨNG — thứ không bao giờ xuất hiện trong dữ liệu bán hàng:
//
//	Chỉ nhìn doanh số:  "Áo khoác bán 200 chiếc" → nhu cầu là 200
//	Thực tế:            bán 200, hết hàng từ tuần 3, 1.500 lượt tìm sau
//	                    khi hết, 400 lượt đăng ký báo hàng → gần 800
//
// Lập kế hoạch chỉ dựa vào doanh số sẽ LIÊN TỤC SẢN XUẤT THIẾU đúng những
// mặt hàng bán chạy nhất.
package supplychain

import "context"

// API là hợp đồng công khai của module supply-chain.
type API interface {
	// RecordSignal ghi một tín hiệu nhu cầu.
	//
	// Gọi trực tiếp chỉ dùng cho tín hiệu KHÔNG có event tương ứng — ví dụ
	// tìm kiếm không ra kết quả, phát sinh ở tầng giao diện.
	//
	// Với những việc đã có event (thêm giỏ, đặt hàng), module này LẮNG
	// NGHE thay vì được gọi: bên phát không cần biết supply-chain tồn tại.
	RecordSignal(ctx context.Context, req SignalRequest) error

	// RecordSignals ghi nhiều tín hiệu trong một lượt.
	RecordSignals(ctx context.Context, reqs []SignalRequest) error

	// CountSignals đếm tín hiệu theo loại trong một khoảng thời gian.
	//
	// Ở MVP đây là công cụ GIÁM SÁT, không phải báo cáo nghiệp vụ: con số
	// bằng 0 kéo dài nghĩa là đường ghi tín hiệu đã hỏng, và mỗi ngày im
	// lặng là một ngày dữ liệu mất vĩnh viễn.
	//
	// from/to rỗng nghĩa là không giới hạn.
	CountSignals(ctx context.Context, from, to string) (map[string]int, error)
}

// SignalRequest là dữ liệu ghi một tín hiệu.
type SignalRequest struct {
	// Type: VIEW, SEARCH, SEARCH_NO_RESULT, CLICK, ADD_TO_CART, WISHLIST,
	// ORDER, STOCKOUT, RETURN, NOTIFY_REQUEST.
	Type string

	// Ít nhất MỘT trong bốn trường sau phải có giá trị.
	//
	// SEARCH_NO_RESULT thì KHÔNG có SKUID — đó chính là ý nghĩa của nó.
	SKUID      string
	ProductID  string
	CategoryID string
	SearchTerm string

	// Quantity: số lượng liên quan. Bỏ trống = 1.
	//
	// Với STOCKOUT, đây là số lượng KHÔNG đáp ứng được — con số quan trọng
	// nhất của tín hiệu đó.
	Quantity int

	// OccurredAt định dạng RFC3339. Bỏ trống = thời điểm hiện tại.
	//
	// Là thời điểm NGHIỆP VỤ, khác thời điểm ghi: tín hiệu đi qua outbox
	// nên có độ trễ, và tổng hợp theo tuần mà dùng thời điểm ghi sẽ đẩy
	// nhầm tín hiệu cuối tuần sang tuần sau.
	OccurredAt string

	SourceType string
	SourceID   string

	Metadata map[string]string
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrInvalidInput = errInvalidInput{}
)

type errInvalidInput struct{}

func (errInvalidInput) Error() string { return "supplychain: dữ liệu không hợp lệ" }

// ---------------------------------------------------------------- Loại tín hiệu

const (
	SignalView           = "VIEW"
	SignalSearch         = "SEARCH"
	SignalSearchNoResult = "SEARCH_NO_RESULT"
	SignalClick          = "CLICK"
	SignalAddToCart      = "ADD_TO_CART"
	SignalWishlist       = "WISHLIST"
	SignalOrder          = "ORDER"
	SignalStockout       = "STOCKOUT"
	SignalReturn         = "RETURN"
	SignalNotifyRequest  = "NOTIFY_REQUEST"
)
