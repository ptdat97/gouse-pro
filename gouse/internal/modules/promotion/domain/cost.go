package domain

import (
	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// AllocateCost chia số tiền khuyến mãi cho các bên phải chịu.
//
// # Đây là câu trả lời cho vấn đề cốt lõi của module
//
//	Khách dùng mã giảm 50.000đ cho đơn của Seller A. AI CHỊU 50.000đ?
//
// Kết quả phải được ĐÓNG BĂNG vào đơn hàng (nguyên tắc P9). Tỷ lệ chia có
// thể đổi khi thỏa thuận với seller thay đổi; nếu đối soát đọc tỷ lệ
// HIỆN TẠI thay vì tỷ lệ lúc bán, số tiền seller đã nhận tháng trước sẽ
// tính ra khác — và không ai giải thích được chênh lệch.
//
// # Tổng luôn bằng ĐÚNG số tiền giảm
//
// Đây là bất biến quan trọng nhất của hàm này. Lệch một đồng là một khoản
// KHÔNG AI CHỊU, và nó xuất hiện ở mọi đơn dùng mã — cuối tháng thành con
// số không nhỏ mà không có bút toán nào giải thích.
//
// Dùng money.Allocate để phần dư của phép chia được rải cho bên đầu tiên
// thay vì biến mất.
func AllocateCost(
	discount money.Money, bearer CostBearer,
	platformBPS, sellerBPS int32, sellerID ids.ID,
) ([]CostAllocation, error) {
	if discount.IsNegative() {
		return nil, ErrInvalidInput
	}
	if discount.IsZero() {
		return nil, nil
	}

	switch bearer {
	case BearerPlatform:
		return []CostAllocation{{
			Bearer: BearerPlatform,
			Amount: discount,
		}}, nil

	case BearerSeller:
		// Khuyến mãi do seller chịu mà không biết seller nào là dữ liệu
		// vô nghĩa: khoản tiền này phải trừ vào số tiền MỘT gian hàng cụ
		// thể nhận được.
		if sellerID.IsZero() {
			return nil, ErrInvalidInput
		}
		return []CostAllocation{{
			Bearer:   BearerSeller,
			SellerID: sellerID,
			Amount:   discount,
		}}, nil

	case BearerShared:
		if sellerID.IsZero() {
			return nil, ErrInvalidInput
		}
		if platformBPS+sellerBPS != 10000 {
			return nil, ErrInvalidInput
		}

		parts, err := discount.Allocate([]int64{int64(platformBPS), int64(sellerBPS)})
		if err != nil {
			return nil, err
		}

		return []CostAllocation{
			{Bearer: BearerPlatform, Amount: parts[0]},
			{Bearer: BearerSeller, SellerID: sellerID, Amount: parts[1]},
		}, nil
	}

	return nil, ErrInvalidInput
}

// AllocateCostFor là AllocateCost dùng cấu hình của một khuyến mãi.
func (p *Promotion) AllocateCostFor(
	discount money.Money, orderSellerID ids.ID,
) ([]CostAllocation, error) {
	// Khuyến mãi gắn với gian hàng thì chi phí thuộc về chính gian hàng
	// đó; khuyến mãi toàn sàn thì thuộc về gian hàng của đơn hàng.
	sellerID := p.sellerID
	if sellerID.IsZero() {
		sellerID = orderSellerID
	}

	return AllocateCost(
		discount, p.costBearer,
		p.platformShareBPS.Value(), p.sellerShareBPS.Value(),
		sellerID)
}
