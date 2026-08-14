// Package domain chứa mô hình nghiệp vụ của module supply-chain.
//
// PHẠM VI MVP: CHỈ GHI TÍN HIỆU NHU CẦU. Chưa có dự báo, chưa có lập kế
// hoạch, chưa có giao diện — những thứ đó ở Phase 3.
//
// VÌ SAO GHI TỪ MVP DÙ CHƯA AI DÙNG:
//
//	DỮ LIỆU LỊCH SỬ KHÔNG TẠO NGƯỢC ĐƯỢC.
//
// Tới Phase 3 mà không có dữ liệu hành vi của 12 tháng trước thì không dự
// báo được gì. Không có cách nào dựng lại "tháng 3 có bao nhiêu người tìm
// áo khoác dạ mà không thấy".
//
// SAI LẦM KINH ĐIỂN mà module này sinh ra để tránh:
//
//	Chỉ nhìn doanh số:  "Áo khoác bán 200 chiếc" → nhu cầu là 200
//
//	Thực tế:            bán 200, HẾT HÀNG từ tuần 3
//	                    1.500 lượt tìm sau khi hết
//	                    400 lượt đăng ký báo có hàng
//	                    → nhu cầu thật gần 800
//
// Lập kế hoạch chỉ dựa vào doanh số lịch sử sẽ LIÊN TỤC SẢN XUẤT THIẾU
// đúng những mặt hàng bán chạy nhất.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrNoSubject    = errors.New("supplychain: tín hiệu phải chỉ tới một sản phẩm, danh mục hoặc từ khóa")
	ErrInvalidType  = errors.New("supplychain: loại tín hiệu không hợp lệ")
	ErrInvalidQty   = errors.New("supplychain: số lượng phải lớn hơn 0")
	ErrNoOccurredAt = errors.New("supplychain: tín hiệu phải có thời điểm xảy ra")
)

// SignalType là loại tín hiệu nhu cầu.
type SignalType string

const (
	SignalView SignalType = "VIEW"

	// SignalSearch là tìm kiếm CÓ kết quả.
	SignalSearch SignalType = "SEARCH"

	// SignalSearchNoResult là tìm kiếm KHÔNG có kết quả.
	//
	// MỘT TRONG BA TÍN HIỆU GIÁ TRỊ NHẤT: nó đo nhu cầu mà nền tảng không
	// đáp ứng được. Không bao giờ xuất hiện trong dữ liệu bán hàng.
	SignalSearchNoResult SignalType = "SEARCH_NO_RESULT"

	SignalClick SignalType = "CLICK"

	// SignalAddToCart mạnh hơn lượt xem rất nhiều: khách đã quyết định
	// muốn món này, chỉ chưa trả tiền.
	SignalAddToCart SignalType = "ADD_TO_CART"

	// SignalWishlist là ý định mua rõ ràng, chỉ chưa đúng thời điểm.
	SignalWishlist SignalType = "WISHLIST"

	// SignalOrder là tín hiệu chắc chắn nhất — khách đã trả tiền.
	SignalOrder SignalType = "ORDER"

	// SignalStockout là hết hàng.
	//
	// TÍN HIỆU GIÁ TRỊ THỨ HAI: mỗi lần hết hàng là một lần nhu cầu có
	// thật bị bỏ lỡ, và nó biến mất khỏi mọi báo cáo doanh số.
	SignalStockout SignalType = "STOCKOUT"

	// SignalReturn kèm lý do hoàn trong metadata.
	//
	// Lý do hoàn là đầu vào để sửa bảng size và mô tả sản phẩm — với thời
	// trang, đây là dữ liệu chất lượng chứ không chỉ là chi phí.
	SignalReturn SignalType = "RETURN"

	// SignalNotifyRequest là đăng ký báo khi có hàng.
	//
	// TÍN HIỆU GIÁ TRỊ THỨ BA: khách chủ động để lại dấu vết nói "tôi muốn
	// mua món này". Không tín hiệu nào rõ ràng hơn.
	SignalNotifyRequest SignalType = "NOTIFY_REQUEST"
)

// IsUnmetDemand cho biết tín hiệu này có đo NHU CẦU CHƯA ĐÁP ỨNG không.
//
// Ba loại này là lý do module tồn tại từ MVP. Chúng không xuất hiện trong
// dữ liệu bán hàng, nên nếu không ghi lại ngay thì mất vĩnh viễn.
func (t SignalType) IsUnmetDemand() bool {
	return t == SignalSearchNoResult || t == SignalStockout || t == SignalNotifyRequest
}

// IsValid kiểm tra loại tín hiệu có nằm trong danh mục không.
func (t SignalType) IsValid() bool {
	switch t {
	case SignalView, SignalSearch, SignalSearchNoResult, SignalClick,
		SignalAddToCart, SignalWishlist, SignalOrder, SignalStockout,
		SignalReturn, SignalNotifyRequest:
		return true
	}
	return false
}

// Signal là MỘT QUAN SÁT về nhu cầu tại một thời điểm.
//
// BẤT BIẾN: tín hiệu mô tả một thời điểm đã qua. Sửa nó nghĩa là sửa lịch
// sử, và toàn bộ giá trị của bảng này nằm ở chỗ lịch sử đáng tin.
//
// Vì vậy struct không có setter nào, và kho lưu trữ chỉ có Append.
type Signal struct {
	signalType SignalType

	// Ba trường định vị. Ít nhất một phải có giá trị.
	//
	// SEARCH_NO_RESULT thì KHÔNG có skuID — đó chính là ý nghĩa của nó.
	skuID      ids.ID
	productID  ids.ID
	categoryID ids.ID
	searchTerm string

	quantity int

	// occurredAt là thời điểm NGHIỆP VỤ, khác thời điểm ghi.
	//
	// Tín hiệu đi qua outbox nên có độ trễ vài giây. Tổng hợp theo tuần mà
	// dùng thời điểm ghi sẽ đẩy nhầm tín hiệu cuối tuần sang tuần sau.
	occurredAt time.Time

	// Nguồn gốc, để truy vết ngược khi con số trông lạ.
	sourceType string
	sourceID   ids.ID

	// metadata giữ phần đặc thù theo loại: lý do hoàn hàng, kênh phát sinh.
	metadata map[string]string
}

type NewSignalParams struct {
	Type SignalType

	SKUID      ids.ID
	ProductID  ids.ID
	CategoryID ids.ID
	SearchTerm string

	Quantity   int
	OccurredAt time.Time

	SourceType string
	SourceID   ids.ID

	Metadata map[string]string
}

// NewSignal tạo một tín hiệu nhu cầu.
func NewSignal(p NewSignalParams) (*Signal, error) {
	if !p.Type.IsValid() {
		return nil, ErrInvalidType
	}

	term := strings.TrimSpace(p.SearchTerm)

	// Tín hiệu phải chỉ vào MỘT THỨ GÌ ĐÓ. Không có gì thì nó không nói
	// lên điều gì và chỉ làm phồng bảng ghi nhiều nhất hệ thống.
	if p.SKUID.IsZero() && p.ProductID.IsZero() && p.CategoryID.IsZero() && term == "" {
		return nil, ErrNoSubject
	}

	qty := p.Quantity
	if qty == 0 {
		qty = 1
	}
	if qty < 0 {
		return nil, ErrInvalidQty
	}

	if p.OccurredAt.IsZero() {
		return nil, ErrNoOccurredAt
	}

	return &Signal{
		signalType: p.Type,
		skuID:      p.SKUID,
		productID:  p.ProductID,
		categoryID: p.CategoryID,
		searchTerm: term,
		quantity:   qty,
		occurredAt: p.OccurredAt,
		sourceType: strings.TrimSpace(p.SourceType),
		sourceID:   p.SourceID,
		metadata:   p.Metadata,
	}, nil
}

func (s *Signal) Type() SignalType      { return s.signalType }
func (s *Signal) SKUID() ids.ID         { return s.skuID }
func (s *Signal) ProductID() ids.ID     { return s.productID }
func (s *Signal) CategoryID() ids.ID    { return s.categoryID }
func (s *Signal) SearchTerm() string    { return s.searchTerm }
func (s *Signal) Quantity() int         { return s.quantity }
func (s *Signal) OccurredAt() time.Time { return s.occurredAt }
func (s *Signal) SourceType() string    { return s.sourceType }
func (s *Signal) SourceID() ids.ID      { return s.sourceID }

// Metadata trả bản sao để không ai sửa được bên trong.
func (s *Signal) Metadata() map[string]string {
	if s.metadata == nil {
		return nil
	}
	out := make(map[string]string, len(s.metadata))
	for k, v := range s.metadata {
		out[k] = v
	}
	return out
}

// IsUnmetDemand cho biết tín hiệu này đo nhu cầu chưa đáp ứng.
func (s *Signal) IsUnmetDemand() bool { return s.signalType.IsUnmetDemand() }
