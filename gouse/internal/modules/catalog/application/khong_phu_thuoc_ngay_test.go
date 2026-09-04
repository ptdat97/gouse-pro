package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
)

// TestKhongPhuThuocNgayChay: bài kiểm chứng rằng phép tìm giấy sắp hết hạn
// cho CÙNG kết quả dù chạy vào ngày nào.
func TestKhongPhuThuocNgayChay(t *testing.T) {
	for _, moc := range []time.Time{
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
		time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		ctx := context.Background()
		svc, clock := newService(t)
		clock.t = moc

		seller := ids.MustNew(ids.PrefixSeller)
		b, _ := svc.CreateBrand(ctx, application.CreateBrandInput{
			Name: "A", Slug: "a"})
		a, _ := svc.GrantAuthorization(ctx, application.GrantAuthorizationInput{
			BrandID: b.ID(), SellerID: seller,
			DocumentURL: "https://cdn/x.pdf",
			ValidFrom:   moc.Add(-24 * time.Hour),
			ValidUntil:  moc.Add(20 * 24 * time.Hour),
		})
		if _, err := svc.ApproveAuthorization(ctx, a.ID(), "nv.hoa"); err != nil {
			t.Fatalf("duyệt: %v", err)
		}

		got, err := svc.FindExpiringAuthorizations(ctx, 30)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("mốc %s: mong 1 giấy sắp hết hạn, nhận %d — "+
				"kết quả KHÔNG được phụ thuộc ngày chạy test",
				moc.Format("2006-01-02"), len(got))
		}
	}
}
