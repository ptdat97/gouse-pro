package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

func sources(sellerIDs ...string) []domain.Source {
	out := make([]domain.Source, 0, len(sellerIDs))
	for _, id := range sellerIDs {
		out = append(out, domain.Source{SellerID: id})
	}
	return out
}

// TestPhiTinhTHEO TUNG NGUON — đây là toàn bộ điểm của P3-8.
//
// # Lỗi đang sửa
//
// Biểu phí cũ nằm trong checkout và thu MỘT lần cho cả đơn, bất kể hàng
// đến từ mấy nguồn. Nhưng hàng của ba nhà bán nằm ở ba kho khác nhau nên
// nó đi thành BA kiện và tốn ba lần phí.
//
// Nghĩa là nền tảng bù phần chênh trên mọi đơn nhiều nguồn — và bù nhiều
// nhất đúng ở loại đơn mà cái chợ tồn tại để tạo ra. Không lỗi nào báo,
// không con số nào âm; nó chỉ hiện ra ở bảng lãi lỗ.
func TestTheFeeIsChargedPerSource(t *testing.T) {
	for _, tt := range []struct {
		ten      string
		sources  []domain.Source
		mongTong int64
	}{
		{"một nhà bán", sources("sel_a"), 30_000},
		{"hai nhà bán", sources("sel_a", "sel_b"), 60_000},
		{"ba nhà bán", sources("sel_a", "sel_b", "sel_c"), 90_000},
	} {
		t.Run(tt.ten, func(t *testing.T) {
			got, err := domain.EstimateShippingFees(
				domain.ShipStandard, tt.sources, money.VND, domain.DefaultFeeSchedule)
			if err != nil {
				t.Fatalf("EstimateShippingFees: %v", err)
			}
			if got.Tong.Amount() != tt.mongTong {
				t.Errorf("tổng phí = %d, mong %d — phí KHÔNG theo số nguồn "+
					"nghĩa là nền tảng bù phần chênh trên mọi đơn nhiều nhà bán",
					got.Tong.Amount(), tt.mongTong)
			}
			if len(got.PerSource) != len(tt.sources) {
				t.Errorf("chi tiết có %d dòng, mong %d",
					len(got.PerSource), len(tt.sources))
			}
		})
	}
}

// TestPerSourceDetailCarriesTheDeliveryTime — mục 7 của đặc tả yêu cầu hiển thị
// thời gian giao RIÊNG cho từng nhóm hàng, không gộp thành một con số.
//
// "Khách cần biết món nào đến trước." Gộp lại là bỏ mất thông tin đó, và
// không lấy lại được ở tầng trên.
func TestPerSourceDetailCarriesTheDeliveryTime(t *testing.T) {
	got, err := domain.EstimateShippingFees(
		domain.ShipExpress, sources("sel_a", "sel_b"), money.VND, domain.DefaultFeeSchedule)
	if err != nil {
		t.Fatalf("EstimateShippingFees: %v", err)
	}

	for i, p := range got.PerSource {
		if p.SellerID == "" {
			t.Errorf("dòng %d không có mã nhà bán — trang không ghép được "+
				"phí với nhóm hàng nào", i)
		}
		if p.EstimatedDays <= 0 {
			t.Errorf("dòng %d có số ngày = %d, phải > 0", i, p.EstimatedDays)
		}
	}
}

// TestExpressCostsMoreAndArrivesSooner — nếu hai phương thức cho cùng phí và cùng
// thời gian thì việc cho khách chọn là vô nghĩa.
func TestExpressCostsMoreAndArrivesSooner(t *testing.T) {
	tc, err := domain.EstimateShippingFees(domain.ShipStandard, sources("sel_a"), money.VND, domain.DefaultFeeSchedule)
	if err != nil {
		t.Fatalf("tiêu chuẩn: %v", err)
	}
	express, err := domain.EstimateShippingFees(domain.ShipExpress, sources("sel_a"), money.VND, domain.DefaultFeeSchedule)
	if err != nil {
		t.Fatalf("nhanh: %v", err)
	}

	if express.Tong.Amount() <= tc.Tong.Amount() {
		t.Errorf("giao nhanh %d không đắt hơn tiêu chuẩn %d",
			express.Tong.Amount(), tc.Tong.Amount())
	}
	if express.PerSource[0].EstimatedDays >= tc.PerSource[0].EstimatedDays {
		t.Errorf("giao nhanh %d ngày không nhanh hơn tiêu chuẩn %d ngày",
			express.PerSource[0].EstimatedDays, tc.PerSource[0].EstimatedDays)
	}
}

// TestNoSourcesMeansZeroFee — không có gì để giao thì không thu.
func TestNoSourcesMeansZeroFee(t *testing.T) {
	got, err := domain.EstimateShippingFees(domain.ShipStandard, nil, money.VND, domain.DefaultFeeSchedule)
	if err != nil {
		t.Fatalf("EstimateShippingFees: %v", err)
	}
	if got.Tong.Amount() != 0 {
		t.Errorf("tổng phí = %d cho đơn không có nguồn nào, mong 0",
			got.Tong.Amount())
	}
}

// TestAnUnknownShipMethodIsRejected — tập ĐÓNG.
//
// Rơi về một mức mặc định là đoán tiền khách phải trả.
func TestAnUnknownShipMethodIsRejected(t *testing.T) {
	_, err := domain.EstimateShippingFees(
		domain.ShipMethod("TAU_VU_TRU"), sources("sel_a"), money.VND, domain.DefaultFeeSchedule)
	if !errors.Is(err, domain.ErrInvalidShipMethod) {
		t.Fatalf("lỗi = %v, mong ErrInvalidShipMethod", err)
	}
}

// TestPhiGiuNGUYEN DON VI TIEN TE của đơn.
//
// Trả phí bằng VND cho một đơn tính bằng USD thì phép cộng vào tổng đơn
// hoặc lỗi, hoặc tệ hơn: cộng hai con số khác đơn vị thành một số vô nghĩa.
func TestTheFeeKeepsTheCurrency(t *testing.T) {
	got, err := domain.EstimateShippingFees(domain.ShipStandard, sources("sel_a"), money.USD, domain.DefaultFeeSchedule)
	if err != nil {
		t.Fatalf("EstimateShippingFees: %v", err)
	}
	if got.Tong.Currency() != money.USD {
		t.Errorf("đơn vị = %s, mong USD", got.Tong.Currency())
	}
}
