package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

func newAuth(t *testing.T, from, until time.Time) *domain.BrandAuthorization {
	t.Helper()
	a, err := domain.NewBrandAuthorization(domain.NewAuthorizationParams{
		BrandID:     ids.MustNew(ids.PrefixBrand),
		SellerID:    ids.MustNew(ids.PrefixSeller),
		DocumentURL: "https://cdn.example.com/uy-quyen.pdf",
		ValidFrom:   from,
		ValidUntil:  until,
	})
	if err != nil {
		t.Fatalf("tạo ủy quyền lỗi: %v", err)
	}
	return a
}

func TestAuthorizationRequiresDocument(t *testing.T) {
	// Không có giấy tờ thì không có ủy quyền — đây là bằng chứng pháp lý
	// khi chủ thương hiệu khiếu nại hàng giả.
	now := time.Now().UTC()
	_, err := domain.NewBrandAuthorization(domain.NewAuthorizationParams{
		BrandID:    ids.MustNew(ids.PrefixBrand),
		SellerID:   ids.MustNew(ids.PrefixSeller),
		ValidFrom:  now,
		ValidUntil: now.Add(365 * 24 * time.Hour),
	})
	if !errors.Is(err, domain.ErrMissingDocument) {
		t.Fatalf("thiếu giấy tờ phải lỗi, nhận %v", err)
	}
}

func TestAuthorizationRejectsInvalidDateRange(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewBrandAuthorization(domain.NewAuthorizationParams{
		BrandID:     ids.MustNew(ids.PrefixBrand),
		SellerID:    ids.MustNew(ids.PrefixSeller),
		DocumentURL: "https://x/y.pdf",
		ValidFrom:   now,
		ValidUntil:  now.Add(-time.Hour), // trước ngày hiệu lực
	})
	if !errors.Is(err, domain.ErrInvalidDateRange) {
		t.Fatalf("khoảng ngày sai phải lỗi, nhận %v", err)
	}
}

// TestAuthorizationValidityWindow là test quan trọng nhất của file.
//
// Giấy ủy quyền HẾT HẠN phải TỰ ĐỘNG chặn việc tạo offer mới — không cần
// ai nhớ thu hồi thủ công. Đây là cơ chế chống hàng giả.
func TestAuthorizationValidityWindow(t *testing.T) {
	base := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	from := base
	until := base.Add(30 * 24 * time.Hour)

	a := newAuth(t, from, until)

	// Chưa duyệt → chưa có hiệu lực dù đang trong khoảng ngày
	if a.IsValidAt(base.Add(24 * time.Hour)) {
		t.Error("giấy CHƯA DUYỆT không được coi là có hiệu lực")
	}

	if err := a.Approve("nv.hoa", base); err != nil {
		t.Fatalf("duyệt lỗi: %v", err)
	}

	cases := []struct {
		name  string
		at    time.Time
		valid bool
	}{
		{"trước ngày hiệu lực", from.Add(-time.Hour), false},
		{"đúng ngày hiệu lực", from, true},
		{"giữa khoảng", from.Add(15 * 24 * time.Hour), true},
		{"đúng lúc hết hạn", until, false},
		{"sau khi hết hạn", until.Add(time.Hour), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.IsValidAt(tc.at); got != tc.valid {
				t.Errorf("IsValidAt(%v) = %v, mong %v", tc.at, got, tc.valid)
			}
		})
	}
}

func TestAuthorizationRevokeBlocksImmediately(t *testing.T) {
	base := time.Now().UTC()
	a := newAuth(t, base, base.Add(365*24*time.Hour))
	_ = a.Approve("nv.hoa", base)

	if !a.IsValidAt(base) {
		t.Fatal("giấy đã duyệt phải có hiệu lực")
	}

	a.Revoke()

	// Thu hồi có hiệu lực NGAY, không chờ hết hạn — cần cho tình huống
	// phát hiện hàng giả và phải chặn ngay lập tức.
	if a.IsValidAt(base) {
		t.Error("giấy đã THU HỒI phải mất hiệu lực ngay")
	}
	// Và không duyệt lại được
	if err := a.Approve("nv.tuan", base); err == nil {
		t.Error("không được duyệt lại giấy đã thu hồi")
	}
}

func TestAuthorizationExpiryWarning(t *testing.T) {
	// Cảnh báo TRƯỚC khi hết hạn để seller kịp gia hạn — nếu chỉ chặn
	// khi đã hết hạn, seller mất doanh số đột ngột.
	base := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	a := newAuth(t, base, base.Add(20*24*time.Hour))
	_ = a.Approve("nv.hoa", base)

	if !a.ExpiresWithin(30*24*time.Hour, base) {
		t.Error("giấy hết hạn sau 20 ngày phải cảnh báo trong ngưỡng 30 ngày")
	}
	if a.ExpiresWithin(10*24*time.Hour, base) {
		t.Error("giấy hết hạn sau 20 ngày KHÔNG nên cảnh báo ở ngưỡng 10 ngày")
	}

	// Giấy đã hết hạn không còn trong diện "sắp hết hạn"
	after := base.Add(25 * 24 * time.Hour)
	if a.ExpiresWithin(30*24*time.Hour, after) {
		t.Error("giấy ĐÃ hết hạn không phải 'sắp hết hạn'")
	}
}
