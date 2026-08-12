package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrEmptySKUCode   = errors.New("product: mã SKU không được rỗng")
	ErrNegativeWeight = errors.New("product: trọng lượng không được âm")
	ErrNegativeDim    = errors.New("product: kích thước không được âm")
)

// SKUStatus là trạng thái của một SKU.
//
// Chỉ hai giá trị: SKU hoặc còn dùng được, hoặc ngừng vĩnh viễn. Không có
// trạng thái "tạm ngừng" — hết hàng tạm thời là việc của module inventory,
// không phải thuộc tính của định danh hàng hóa.
type SKUStatus string

const (
	SKUActive       SKUStatus = "ACTIVE"
	SKUDiscontinued SKUStatus = "DISCONTINUED"
)

// Dimensions là kích thước gói hàng, đơn vị milimét.
//
// Số nguyên chứ không phải số thực: đơn vị mm đủ chính xác cho vận chuyển,
// và số thực gây sai số tích lũy khi tính thể tích xếp kho.
type Dimensions struct {
	LengthMM int
	WidthMM  int
	HeightMM int
}

// IsZero cho biết kích thước chưa được khai báo.
func (d Dimensions) IsZero() bool {
	return d.LengthMM == 0 && d.WidthMM == 0 && d.HeightMM == 0
}

// VolumeMM3 tính thể tích, dùng cho xếp kho và tính phí theo thể tích.
func (d Dimensions) VolumeMM3() int {
	return d.LengthMM * d.WidthMM * d.HeightMM
}

// SKU là đơn vị lưu kho — định danh hàng hóa CHUNG.
//
// SKU KHÔNG thuộc về seller nào. Đây là điểm mấu chốt cho mô hình
// marketplace: nó cho phép biết "ba seller đang bán cùng một món hàng",
// nền tảng để so giá và chọn nhà bán tốt nhất.
//
// Nếu mỗi seller có mã hàng riêng, không bao giờ ghép được các offer lại
// với nhau và nền tảng chỉ là tập hợp các gian hàng rời rạc.
type SKU struct {
	id        ids.ID
	variantID ids.ID

	code    string
	barcode string

	// weightGram và dimensions cần cho tính phí vận chuyển và xếp kho —
	// không phải thông tin phụ (docs/04-modules/product.md mục 8).
	weightGram int
	dimensions Dimensions

	status SKUStatus

	createdAt time.Time
	updatedAt time.Time
}

type NewSKUParams struct {
	Code       string
	Barcode    string
	WeightGram int
	Dimensions Dimensions
	Now        time.Time
}

// NewSKU tạo SKU mới.
func NewSKU(p NewSKUParams) (*SKU, error) {
	code := normalizeSKUCode(p.Code)
	if code == "" {
		return nil, ErrEmptySKUCode
	}
	if p.WeightGram < 0 {
		return nil, ErrNegativeWeight
	}
	if p.Dimensions.LengthMM < 0 || p.Dimensions.WidthMM < 0 || p.Dimensions.HeightMM < 0 {
		return nil, ErrNegativeDim
	}

	id, err := ids.New(ids.PrefixSKU)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &SKU{
		id:         id,
		code:       code,
		barcode:    strings.TrimSpace(p.Barcode),
		weightGram: p.WeightGram,
		dimensions: p.Dimensions,
		status:     SKUActive,
		createdAt:  now,
		updatedAt:  now,
	}, nil
}

// normalizeSKUCode chuẩn hóa mã SKU: viết hoa, bỏ khoảng trắng.
//
// Mã SKU được người thật gõ vào kho và quét ở nhiều nơi. Không chuẩn hóa
// thì "sm-lin-wht-m" và "SM-LIN-WHT-M" thành hai mặt hàng khác nhau, và
// tồn kho sẽ tách làm đôi một cách âm thầm.
func normalizeSKUCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// RestoreSKUParams dựng lại SKU từ kho lưu trữ.
type RestoreSKUParams struct {
	ID         ids.ID
	VariantID  ids.ID
	Code       string
	Barcode    string
	WeightGram int
	Dimensions Dimensions
	Status     SKUStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// RestoreSKU dựng lại SKU mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreSKU(p RestoreSKUParams) *SKU {
	return &SKU{
		id:         p.ID,
		variantID:  p.VariantID,
		code:       p.Code,
		barcode:    p.Barcode,
		weightGram: p.WeightGram,
		dimensions: p.Dimensions,
		status:     p.Status,
		createdAt:  p.CreatedAt,
		updatedAt:  p.UpdatedAt,
	}
}

func (s *SKU) ID() ids.ID             { return s.id }
func (s *SKU) VariantID() ids.ID      { return s.variantID }
func (s *SKU) Code() string           { return s.code }
func (s *SKU) Barcode() string        { return s.barcode }
func (s *SKU) WeightGram() int        { return s.weightGram }
func (s *SKU) Dimensions() Dimensions { return s.dimensions }
func (s *SKU) Status() SKUStatus      { return s.status }
func (s *SKU) CreatedAt() time.Time   { return s.createdAt }
func (s *SKU) UpdatedAt() time.Time   { return s.updatedAt }

// IsSellable cho biết SKU còn dùng để bán được không.
//
// Đây KHÔNG phải câu trả lời "còn hàng không" — tồn kho thuộc module
// inventory. Ở đây chỉ trả lời "mặt hàng này còn được kinh doanh không".
func (s *SKU) IsSellable() bool { return s.status == SKUActive }

// CanShip cho biết đã đủ thông tin để tính phí vận chuyển chưa.
//
// Thiếu trọng lượng thì hãng vận chuyển tính theo mặc định, thường CAO hơn
// thực tế — chi phí này âm thầm ăn vào biên lợi nhuận.
func (s *SKU) CanShip() bool {
	return s.weightGram > 0 && !s.dimensions.IsZero()
}

// SetShippingInfo cập nhật thông tin vận chuyển.
func (s *SKU) SetShippingInfo(weightGram int, d Dimensions, now time.Time) error {
	if weightGram < 0 {
		return ErrNegativeWeight
	}
	if d.LengthMM < 0 || d.WidthMM < 0 || d.HeightMM < 0 {
		return ErrNegativeDim
	}
	s.weightGram = weightGram
	s.dimensions = d
	s.touch(now)
	return nil
}

// Discontinue ngừng kinh doanh SKU vĩnh viễn.
//
// KHÔNG xóa: đơn hàng cũ trỏ tới SKU này và lịch sử mua hàng phải hiển thị
// được. Xóa SKU làm hỏng dữ liệu đơn hàng đã hoàn tất.
func (s *SKU) Discontinue(now time.Time) {
	s.status = SKUDiscontinued
	s.touch(now)
}

func (s *SKU) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.updatedAt = now
}
