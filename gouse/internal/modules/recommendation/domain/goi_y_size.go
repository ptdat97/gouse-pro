// Package domain là mô hình nghiệp vụ của việc gợi ý.
//
// # Phạm vi đợt đầu: ĐÚNG MỘT phương thức
//
// `size_recommendation` là trường đã khai trong `ProductDetail`, ở một
// endpoint đang chạy, có người dùng thật đọc — nên nó có bên gọi.
//
// Bốn phương thức còn lại của interface gợi ý (sản phẩm tương tự, xu
// hướng, hoàn thiện bộ, nội dung liên quan) KHÔNG có bên gọi nào: đặc tả
// API không khai `similar_products`, không có endpoint xu hướng. Dựng
// chúng bây giờ là tạo thêm một ca của dạng lỗi "khai mà không ai gọi".
package domain

import (
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Outcome là điều khách NÓI RA về một size.
type Outcome string

const (
	// OutcomePurchased: đã mua và không trả vì lý do size.
	//
	// Tín hiệu YẾU. Khách giữ hàng có thể vì vừa, cũng có thể vì ngại trả
	// — nên nó chỉ được dùng khi không có tín hiệu mạnh hơn.
	OutcomePurchased Outcome = "PURCHASED"

	// OutcomeTight: trả hàng vì chật. Lần sau nên LÊN một size.
	OutcomeTight Outcome = "TIGHT"

	// OutcomeLoose: trả hàng vì rộng. Lần sau nên XUỐNG một size.
	OutcomeLoose Outcome = "LOOSE"
)

// Observation là một lần khách nói ra điều gì đó về một size.
type Observation struct {
	Size    string
	Outcome Outcome

	// Seq là thứ tự thời gian: số LỚN hơn là mới hơn.
	Seq int64
}

// ReasonCode là căn cứ của một gợi ý, theo đúng enum của đặc tả API.
type ReasonCode string

const (
	ReasonPurchased ReasonCode = "PREVIOUS_PURCHASE"
	ReasonReturned  ReasonCode = "RETURN_HISTORY"
)

// Suggestion là kết quả gợi ý size.
type Suggestion struct {
	Size       string
	ReasonCode ReasonCode
}

// InferSize chọn size nên gợi ý, hoặc trả false nếu không đủ căn cứ.
//
// # Thứ tự ưu tiên, và vì sao TRẢ HÀNG thắng
//
// Một lần trả hàng vì size là khách CHỦ ĐỘNG bỏ công nói ra rằng size đó
// sai, và sai theo hướng nào. Một lần mua rồi im lặng thì mơ hồ hơn nhiều:
// có thể vừa, có thể họ ngại trả.
//
// Nên quan sát TRẢ HÀNG mới nhất thắng mọi lần mua. Chỉ khi chưa từng có
// phàn nàn nào về size thì mới dùng size đã mua.
//
// # Vì sao cần `sizes` theo THỨ TỰ
//
// "Lên một size" chỉ có nghĩa khi biết scale size của sản phẩm đang xem.
// Thang ấy khác nhau theo loại hàng: S/M/L, 38/39/40, hay Free. Bên gọi —
// module `product` — là nơi biết scale đó, nên nó truyền vào.
//
// # Trả FALSE thay vì đoán
//
// Không có quan sát nào, hoặc size cần gợi ý nằm ngoài scale đang bán, thì
// KHÔNG gợi ý. Đặc tả khai trường này nullable đúng vì thế: một gợi ý sai
// còn tệ hơn không gợi ý, vì nó làm khách chọn nhầm rồi đổ lỗi cho nền
// tảng — và làm tăng đúng tỷ lệ hoàn hàng mà nó sinh ra để giảm.
func InferSize(observation []Observation, sizeScale []string) (Suggestion, bool) {
	scale := normalizeScale(sizeScale)
	if len(scale) == 0 {
		return Suggestion{}, false
	}

	var latestReturn *Observation
	var latestPurchase *Observation
	for i := range observation {
		q := &observation[i]
		switch q.Outcome {
		case OutcomeTight, OutcomeLoose:
			if latestReturn == nil || q.Seq > latestReturn.Seq {
				latestReturn = q
			}
		case OutcomePurchased:
			if latestPurchase == nil || q.Seq > latestPurchase.Seq {
				latestPurchase = q
			}
		}
	}

	if latestReturn != nil {
		step := 1
		if latestReturn.Outcome == OutcomeLoose {
			step = -1
		}
		if s, ok := shiftSize(scale, latestReturn.Size, step); ok {
			return Suggestion{Size: s, ReasonCode: ReasonReturned}, true
		}
		// Đã phàn nàn nhưng không dịch được — ví dụ size đó đã là lớn nhất
		// scale. KHÔNG rơi xuống "size đã mua": khách vừa nói size ấy sai,
		// gợi ý lại chính nó là phớt lờ điều họ nói.
		return Suggestion{}, false
	}

	if latestPurchase != nil {
		if _, ok := position(scale, latestPurchase.Size); ok {
			return Suggestion{Size: normalize(latestPurchase.Size), ReasonCode: ReasonPurchased}, true
		}
	}

	return Suggestion{}, false
}

// shiftSize trả size cách `step` bậc trong scale, nếu còn trong scale.
func shiftSize(scale []string, size string, step int) (string, bool) {
	i, ok := position(scale, size)
	if !ok {
		return "", false
	}
	j := i + step
	if j < 0 || j >= len(scale) {
		return "", false
	}
	return scale[j], true
}

func position(scale []string, size string) (int, bool) {
	s := normalize(size)
	for i, v := range scale {
		if v == s {
			return i, true
		}
	}
	return 0, false
}

// normalizeScale bỏ size rỗng và trùng, GIỮ NGUYÊN thứ tự bên gọi đưa vào.
//
// Thứ tự là thông tin của bên gọi, không phải thứ tự chữ cái: "S, M, L"
// sắp theo bảng chữ cái sẽ thành "L, M, S" và mọi phép "lên một size" đảo
// chiều.
func normalizeScale(sizeScale []string) []string {
	out := make([]string, 0, len(sizeScale))
	seen := make(map[string]bool, len(sizeScale))
	for _, s := range sizeScale {
		v := normalize(s)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalize(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// NewObservation là một quan sát chuẩn bị ghi xuống.
//
// Tách khỏi `Observation` vì hai thứ phục vụ hai chiều: cái này để GHI và mang
// đủ nguồn gốc để chống trùng; `Observation` để ĐỌC và chỉ mang thứ quy tắc
// suy luận cần.
type NewObservation struct {
	CustomerID ids.ID
	BrandID    ids.ID
	Size       string
	Outcome    Outcome

	// SourceType và SourceID cho biết quan sát này đến từ đâu — khóa chống
	// trùng khi event được phát lại.
	SourceType string
	SourceID   ids.ID

	ObservedAt time.Time
}
