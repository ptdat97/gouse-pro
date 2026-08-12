package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

func TestNewBrandValidation(t *testing.T) {
	cases := []struct {
		name    string
		params  domain.NewBrandParams
		wantErr error
	}{
		{
			name:   "hợp lệ",
			params: domain.NewBrandParams{Name: "Thương hiệu A", Slug: "thuong-hieu-a"},
		},
		{
			name:    "tên rỗng",
			params:  domain.NewBrandParams{Name: "  ", Slug: "abc"},
			wantErr: domain.ErrEmptyName,
		},
		{
			name:    "slug có chữ hoa",
			params:  domain.NewBrandParams{Name: "A", Slug: "Thuong-Hieu"},
			wantErr: domain.ErrInvalidSlug,
		},
		{
			name:    "slug có khoảng trắng",
			params:  domain.NewBrandParams{Name: "A", Slug: "thuong hieu"},
			wantErr: domain.ErrInvalidSlug,
		},
		{
			name:    "slug bắt đầu bằng gạch ngang",
			params:  domain.NewBrandParams{Name: "A", Slug: "-abc"},
			wantErr: domain.ErrInvalidSlug,
		},
		{
			name:    "slug rỗng",
			params:  domain.NewBrandParams{Name: "A", Slug: ""},
			wantErr: domain.ErrInvalidSlug,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := domain.NewBrand(tc.params)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("mong lỗi %v, nhận %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("không mong lỗi: %v", err)
			}
			if b.ID().Prefix() != ids.PrefixBrand {
				t.Errorf("ID phải có tiền tố brd_, nhận %q", b.ID())
			}
			if b.Status() != domain.StatusActive {
				t.Errorf("brand mới phải ACTIVE, nhận %q", b.Status())
			}
		})
	}
}

func TestBrandDefaultsToOpenProtection(t *testing.T) {
	// Mặc định OPEN: không chặn seller khi thương hiệu chưa cần bảo vệ.
	// Nếu mặc định là VERIFIED_ONLY, mọi thương hiệu mới đều bị khóa.
	b, err := domain.NewBrand(domain.NewBrandParams{Name: "A", Slug: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ProtectionLevel() != domain.ProtectionOpen {
		t.Errorf("mặc định phải là OPEN, nhận %q", b.ProtectionLevel())
	}
	if b.IsProtected() {
		t.Error("brand OPEN không được coi là protected")
	}
}

func TestBrandProtectionLevels(t *testing.T) {
	b, _ := domain.NewBrand(domain.NewBrandParams{
		Name: "Thương hiệu cao cấp", Slug: "cao-cap",
		ProtectionLevel: domain.ProtectionVerifiedOnly,
	})

	if !b.IsProtected() {
		t.Error("VERIFIED_ONLY phải được coi là protected")
	}

	// Đổi mức bảo vệ
	now := time.Now().UTC()
	if err := b.SetProtectionLevel(domain.ProtectionRestricted, now); err != nil {
		t.Fatalf("đổi mức bảo vệ hợp lệ không được lỗi: %v", err)
	}
	if b.ProtectionLevel() != domain.ProtectionRestricted {
		t.Errorf("mong RESTRICTED, nhận %q", b.ProtectionLevel())
	}

	// Mức không hợp lệ bị từ chối
	if err := b.SetProtectionLevel("KHONG_TON_TAI", now); err == nil {
		t.Error("mức bảo vệ không hợp lệ phải bị từ chối")
	}
}

func TestOwnBrandIdentification(t *testing.T) {
	own, _ := domain.NewBrand(domain.NewBrandParams{
		Name: "Own Brand", Slug: "own", BrandType: domain.BrandTypeOwn,
	})
	third, _ := domain.NewBrand(domain.NewBrandParams{
		Name: "Bên thứ ba", Slug: "third",
	})

	if !own.IsOwnBrand() {
		t.Error("BrandTypeOwn phải là own brand")
	}
	if third.IsOwnBrand() {
		t.Error("mặc định THIRD_PARTY không phải own brand")
	}
}

func TestBrandRestoreSkipsValidation(t *testing.T) {
	// Dữ liệu cũ phải đọc được kể cả khi quy tắc nghiệp vụ đã đổi.
	// Nếu RestoreBrand kiểm tra lại, một thay đổi quy tắc sẽ làm
	// không đọc được bản ghi cũ.
	b := domain.RestoreBrand(domain.RestoreBrandParams{
		ID:   ids.MustNew(ids.PrefixBrand),
		Name: "", // rỗng — không hợp lệ với NewBrand
		Slug: "SLUG_KHONG_HOP_LE",
	})
	if b == nil {
		t.Fatal("RestoreBrand phải luôn thành công")
	}
}
