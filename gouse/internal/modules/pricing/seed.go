package pricing

import (
	"context"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/application"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

// SeedInput là các SKU cần đặt giá.
//
// Giá gắn với SKU có thật từ module product — không thể tự bịa định danh.
type SeedInput struct {
	SKUIDs []string
}

// SeedResult là kết quả nạp dữ liệu mẫu.
type SeedResult struct {
	// PricedSKUID là SKU đã có giá, dùng để gọi thử.
	PricedSKUID string

	// ClearanceSKUID là SKU có cả giá gốc lẫn giá xả hàng — dùng để kiểm
	// chứng quy tắc ưu tiên.
	ClearanceSKUID string
}

// SeedDemo nạp dữ liệu giá mẫu khi dùng kho in-memory.
//
// CHỈ dùng cho môi trường phát triển. Khi có PostgreSQL, việc này chuyển
// sang migration/fixture và hàm này biến mất.
//
// Nạp nhiều LOẠI giá để kiểm chứng được quy tắc ưu tiên: SKU thứ hai có
// cả giá gốc lẫn giá xả hàng, và giá xả hàng phải thắng.
func SeedDemo(ctx context.Context, m *Module, in SeedInput) (SeedResult, error) {
	var out SeedResult
	if len(in.SKUIDs) == 0 {
		return out, nil
	}

	svc := m.svc
	now := svc.Now()

	// SKU đầu: giá gốc kèm giá gạch ngang.
	first, err := ids.Parse(in.SKUIDs[0], ids.PrefixSKU)
	if err != nil {
		return out, fmt.Errorf("sku_id không hợp lệ: %w", err)
	}
	if _, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID:     first,
		PriceType: domain.PriceTypeBase,
		Amount:    money.MustNew(490000, money.VND),
		CompareAt: money.MustNew(590000, money.VND),
		Reason:    domain.ReasonInitial,
	}); err != nil {
		return out, fmt.Errorf("nạp giá gốc: %w", err)
	}
	out.PricedSKUID = first.String()

	// Khung giá ràng buộc seller cho SKU đó.
	if _, err := svc.SetConstraint(ctx, application.SetConstraintInput{
		SKUID:          first,
		MinPrice:       money.MustNew(300000, money.VND),
		MaxPrice:       money.MustNew(800000, money.VND),
		ReferencePrice: money.MustNew(490000, money.VND),
	}); err != nil {
		return out, fmt.Errorf("nạp khung giá: %w", err)
	}

	// SKU thứ hai: giá gốc + giá xả hàng, để kiểm chứng quy tắc ưu tiên.
	if len(in.SKUIDs) > 1 {
		second, err := ids.Parse(in.SKUIDs[1], ids.PrefixSKU)
		if err != nil {
			return out, fmt.Errorf("sku_id không hợp lệ: %w", err)
		}

		if _, err := svc.SetPrice(ctx, application.SetPriceInput{
			SKUID:     second,
			PriceType: domain.PriceTypeBase,
			Amount:    money.MustNew(520000, money.VND),
			Reason:    domain.ReasonInitial,
		}); err != nil {
			return out, fmt.Errorf("nạp giá gốc SKU 2: %w", err)
		}

		if _, err := svc.SetPrice(ctx, application.SetPriceInput{
			SKUID:     second,
			PriceType: domain.PriceTypeClearance,
			Amount:    money.MustNew(299000, money.VND),
			CompareAt: money.MustNew(520000, money.VND),
			Reason:    domain.ReasonSeasonEnd,
			Period:    domain.Period{From: now.AddDate(0, 0, -7)},
		}); err != nil {
			return out, fmt.Errorf("nạp giá xả hàng: %w", err)
		}
		out.ClearanceSKUID = second.String()
	}

	return out, nil
}
