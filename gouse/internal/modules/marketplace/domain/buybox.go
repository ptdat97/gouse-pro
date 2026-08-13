package domain

import (
	"sort"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// BuyBoxWeights là trọng số của công thức chọn buy box.
//
// NGUYÊN TẮC THIẾT KẾ (mục 4 của đặc tả): công thức phải TƯỜNG MINH và
// CÔNG KHAI với seller.
//
// Seller cần hiểu vì sao mình không thắng buy box và làm gì để cải thiện.
// Một mô hình hộp đen tạo tranh chấp không giải quyết được và cảm giác bất
// công — dẫn tới seller rời nền tảng.
//
// CẢNH BÁO VỀ CẠNH TRANH GIÁ: nếu buy box chỉ dựa vào giá thấp nhất, seller
// đua giảm giá tới mức không bền vững và cắt giảm chất lượng dịch vụ. Trọng
// số phải cân bằng giá và chất lượng phục vụ — đó là lý do giá chỉ chiếm
// một nửa, không phải toàn bộ.
type BuyBoxWeights struct {
	Price       int
	Handling    int
	Performance int
}

// DefaultWeights là trọng số mặc định của MVP.
//
// Giá 40%, thời gian giao 30%, hiệu suất seller 30%.
//
// VÌ SAO GIÁ DƯỚI 50%: với giá chiếm một nửa, một offer rẻ hơn 10% thắng
// được offer kém nhất ở CẢ HAI tiêu chí còn lại (52 điểm so với 50). Đó
// đúng là cuộc đua giảm giá mà đặc tả cảnh báo — seller hạ giá tới mức
// không bền vững rồi cắt chất lượng dịch vụ để bù.
//
// Với 40/30/30, chất lượng phục vụ (60%) đủ sức thắng một khoảng chênh giá
// nhỏ, nhưng giá vẫn là yếu tố ĐƠN LẺ nặng nhất — khách vẫn được lợi khi
// seller cạnh tranh giá thật sự.
//
// Đây là quy tắc ĐƠN GIẢN và GIẢI THÍCH ĐƯỢC theo nguyên tắc P14. Con số
// cụ thể nên hiệu chỉnh lại khi có dữ liệu thật về hành vi khách hàng.
var DefaultWeights = BuyBoxWeights{Price: 40, Handling: 30, Performance: 30}

// BuyBoxCandidate là một offer tham gia tranh buy box.
//
// Gộp offer với dữ liệu từ module khác (hiệu suất seller) thành một cấu
// trúc: domain không gọi module khác, tầng application chuẩn bị sẵn.
type BuyBoxCandidate struct {
	Offer *Offer

	// SellerActive: seller có đang hoạt động không.
	//
	// RÀNG BUỘC BẮT BUỘC: seller bị đình chỉ thì offer không được thắng
	// buy box, kể cả khi giá tốt nhất.
	SellerActive bool

	// PerformanceScore trong khoảng [0, 100]. Chưa có module chấm điểm
	// hiệu suất (Phase 2) thì dùng giá trị mặc định.
	PerformanceScore int
}

// DefaultPerformanceScore dùng khi chưa có dữ liệu hiệu suất.
//
// Đặt ở mức trung bình chứ không phải 0: seller mới chưa có lịch sử không
// nên bị phạt như seller có hiệu suất kém thật.
const DefaultPerformanceScore = 50

// BuyBoxResult là kết quả chọn buy box.
type BuyBoxResult struct {
	Winner *Offer

	// Score là điểm của offer thắng, để giải thích cho seller.
	Score int

	// OtherCount là số offer khác cùng tranh.
	OtherCount int
}

// SelectBuyBox chọn offer hiển thị mặc định cho một SKU.
//
// RÀNG BUỘC BẮT BUỘC (mục 4) — ứng viên bị loại nếu:
//   - Offer không ở trạng thái bán được
//   - Seller không hoạt động
//
// Trả về nil nếu không ứng viên nào hợp lệ.
//
// Điểm được tính theo công thức công khai, và trả kèm kết quả để trả lời
// được câu hỏi "vì sao offer của tôi không thắng".
func SelectBuyBox(candidates []BuyBoxCandidate, w BuyBoxWeights) BuyBoxResult {
	eligible := make([]BuyBoxCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Offer == nil || !c.Offer.IsSellable() || !c.SellerActive {
			continue
		}
		eligible = append(eligible, c)
	}
	if len(eligible) == 0 {
		return BuyBoxResult{}
	}

	// Giá thấp nhất và cao nhất để chuẩn hóa điểm giá.
	lowest, highest := eligible[0].Offer.Price(), eligible[0].Offer.Price()
	fastest, slowest := eligible[0].Offer.HandlingTimeHours(), eligible[0].Offer.HandlingTimeHours()
	for _, c := range eligible[1:] {
		if c.Offer.Price().LessThan(lowest) {
			lowest = c.Offer.Price()
		}
		if highest.LessThan(c.Offer.Price()) {
			highest = c.Offer.Price()
		}
		if h := c.Offer.HandlingTimeHours(); h < fastest {
			fastest = h
		} else if h > slowest {
			slowest = h
		}
	}

	type scored struct {
		c     BuyBoxCandidate
		score int
	}
	list := make([]scored, 0, len(eligible))
	for _, c := range eligible {
		list = append(list, scored{c: c, score: score(c, w, lowest, highest, fastest, slowest)})
	}

	// Sắp xếp: điểm cao thắng; hòa thì giá thấp thắng; hòa nữa thì theo id
	// để kết quả ỔN ĐỊNH.
	//
	// Thứ tự ổn định là bắt buộc: buy box nhảy qua lại giữa hai offer
	// ngang điểm sẽ làm khách thấy giá đổi liên tục khi tải lại trang.
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		pi, pj := list[i].c.Offer.Price(), list[j].c.Offer.Price()
		if !pi.Equal(pj) {
			return pi.LessThan(pj)
		}
		return list[i].c.Offer.ID() < list[j].c.Offer.ID()
	})

	return BuyBoxResult{
		Winner:     list[0].c.Offer,
		Score:      list[0].score,
		OtherCount: len(list) - 1,
	}
}

// score tính điểm của một ứng viên theo công thức công khai.
//
// Mỗi thành phần cho điểm 0–100 rồi nhân trọng số, nên tổng điểm nằm trong
// khoảng dễ đọc và giải thích được cho seller.
func score(c BuyBoxCandidate, w BuyBoxWeights, lowest, highest money.Money, fastest, slowest int) int {
	priceScore := normalizeLowerIsBetter(
		c.Offer.Price().Amount(), lowest.Amount(), highest.Amount())

	handlingScore := normalizeLowerIsBetter(
		int64(c.Offer.HandlingTimeHours()), int64(fastest), int64(slowest))

	perf := c.PerformanceScore
	if perf < 0 {
		perf = 0
	} else if perf > 100 {
		perf = 100
	}

	total := w.Price + w.Handling + w.Performance
	if total <= 0 {
		return 0
	}
	return (priceScore*w.Price + handlingScore*w.Handling + perf*w.Performance) / total
}

// normalizeLowerIsBetter cho điểm 0–100, giá trị THẤP hơn được điểm CAO hơn.
//
// Mọi ứng viên bằng nhau thì cùng được 100 — không ai bị phạt vì lý do
// ngoài tầm kiểm soát của họ.
func normalizeLowerIsBetter(v, best, worst int64) int {
	if worst <= best {
		return 100
	}
	// v = best → 100; v = worst → 0.
	return int(100 * (worst - v) / (worst - best))
}

// SKUCandidates gom ứng viên theo SKU, dùng cho tính buy box theo lô.
type SKUCandidates map[ids.ID][]BuyBoxCandidate
