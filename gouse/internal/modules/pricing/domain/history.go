package domain

import (
	"sort"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// ChangeReason là lý do giá thay đổi.
//
// Ghi lý do chứ không chỉ ghi giá mới: khi rà soát thao túng giá, "tăng
// giá rồi giảm ngay trước đợt khuyến mãi" chỉ nhận ra được nếu biết vì sao
// mỗi lần đổi.
type ChangeReason string

const (
	ReasonInitial    ChangeReason = "INITIAL"
	ReasonManual     ChangeReason = "MANUAL"
	ReasonCampaign   ChangeReason = "CAMPAIGN"
	ReasonSeasonEnd  ChangeReason = "SEASON_END"
	ReasonClearance  ChangeReason = "CLEARANCE"
	ReasonCostChange ChangeReason = "COST_CHANGE"
	ReasonCompetitor ChangeReason = "COMPETITOR_MATCH"
)

// PricePoint là một điểm trong lịch sử giá — BẤT BIẾN.
//
// Không có phương thức sửa đổi và mọi trường đều riêng tư, chỉ đọc được
// qua getter. Lịch sử sửa được thì không còn là lịch sử.
//
// VÌ SAO CẦN LỊCH SỬ (mục 6 của đặc tả):
//
//  1. Phát hiện thao túng giá — tăng rồi giảm giả vờ khuyến mãi
//  2. Phân tích độ co giãn của cầu
//  3. Nghĩa vụ minh bạch giá ở một số thị trường
//
// Điểm 3 là lý do không thể bỏ qua: một số nơi yêu cầu "giá thấp nhất
// trong 30 ngày qua" khi quảng cáo giảm giá. Không có lịch sử thì không
// trả lời được, và dữ liệu này KHÔNG tạo ngược được.
type PricePoint struct {
	id        ids.ID
	skuID     ids.ID
	priceType PriceType

	amount    money.Money
	compareAt money.Money

	reason ChangeReason

	// changedBy là người hoặc hệ thống gây ra thay đổi. Rỗng nghĩa là
	// hệ thống tự động.
	changedBy ids.ID

	recordedAt time.Time
}

type NewPricePointParams struct {
	SKUID     ids.ID
	PriceType PriceType
	Amount    money.Money
	CompareAt money.Money
	Reason    ChangeReason
	ChangedBy ids.ID
	Now       time.Time
}

// NewPricePoint ghi một điểm lịch sử giá.
func NewPricePoint(p NewPricePointParams) (*PricePoint, error) {
	if p.SKUID.IsZero() {
		return nil, ErrMissingSKU
	}
	if !p.Amount.IsPositive() {
		return nil, ErrNonPositivePrice
	}

	id, err := ids.New(ids.PrefixPrice)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	reason := p.Reason
	if reason == "" {
		reason = ReasonManual
	}

	return &PricePoint{
		id:         id,
		skuID:      p.SKUID,
		priceType:  p.PriceType,
		amount:     p.Amount,
		compareAt:  p.CompareAt,
		reason:     reason,
		changedBy:  p.ChangedBy,
		recordedAt: now,
	}, nil
}

// RestorePricePointParams dựng lại điểm lịch sử từ kho lưu trữ.
type RestorePricePointParams struct {
	ID         ids.ID
	SKUID      ids.ID
	PriceType  PriceType
	Amount     money.Money
	CompareAt  money.Money
	Reason     ChangeReason
	ChangedBy  ids.ID
	RecordedAt time.Time
}

// RestorePricePoint dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestorePricePoint(p RestorePricePointParams) *PricePoint {
	return &PricePoint{
		id:         p.ID,
		skuID:      p.SKUID,
		priceType:  p.PriceType,
		amount:     p.Amount,
		compareAt:  p.CompareAt,
		reason:     p.Reason,
		changedBy:  p.ChangedBy,
		recordedAt: p.RecordedAt,
	}
}

func (p *PricePoint) ID() ids.ID             { return p.id }
func (p *PricePoint) SKUID() ids.ID          { return p.skuID }
func (p *PricePoint) Type() PriceType        { return p.priceType }
func (p *PricePoint) Amount() money.Money    { return p.amount }
func (p *PricePoint) CompareAt() money.Money { return p.compareAt }
func (p *PricePoint) Reason() ChangeReason   { return p.reason }
func (p *PricePoint) ChangedBy() ids.ID      { return p.changedBy }
func (p *PricePoint) RecordedAt() time.Time  { return p.recordedAt }

// DateRange là khoảng thời gian truy vấn lịch sử.
type DateRange struct {
	From time.Time
	To   time.Time
}

// Contains cho biết thời điểm có nằm trong khoảng không.
//
// Bao gồm From, KHÔNG bao gồm To — cùng quy ước với Period.
func (d DateRange) Contains(t time.Time) bool {
	if !d.From.IsZero() && t.Before(d.From) {
		return false
	}
	if !d.To.IsZero() && !t.Before(d.To) {
		return false
	}
	return true
}

// LowestIn tìm giá THẤP NHẤT trong khoảng thời gian.
//
// Đây là con số một số thị trường bắt buộc công bố khi quảng cáo giảm giá:
// "giá thấp nhất 30 ngày qua". Nếu không có, quảng cáo "giảm 50%" so với
// một mức giá vừa được nâng lên là hành vi bị cấm.
//
// Trả về false nếu không có điểm nào trong khoảng.
func LowestIn(points []*PricePoint, r DateRange) (money.Money, bool) {
	var lowest money.Money
	found := false

	for _, p := range points {
		if p == nil || !r.Contains(p.recordedAt) {
			continue
		}
		if !found {
			lowest, found = p.amount, true
			continue
		}
		if p.amount.LessThan(lowest) {
			lowest = p.amount
		}
	}
	return lowest, found
}

// SortByTime sắp xếp lịch sử theo thời gian tăng dần.
//
// Trả về lát cắt MỚI, không sửa lát cắt đầu vào — bên gọi thường đang giữ
// dữ liệu dùng cho việc khác.
func SortByTime(points []*PricePoint) []*PricePoint {
	out := append([]*PricePoint(nil), points...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].recordedAt.Equal(out[j].recordedAt) {
			// Cùng thời điểm thì xếp theo id để thứ tự ỔN ĐỊNH.
			// ULID sinh theo thời gian nên id lớn hơn là mới hơn.
			return out[i].id < out[j].id
		}
		return out[i].recordedAt.Before(out[j].recordedAt)
	})
	return out
}
