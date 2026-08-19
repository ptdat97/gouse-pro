package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Các test trong file này kiểm tra HỢP ĐỒNG CÔNG KHAI của module — thứ mà
// module khác (marketplace, product) sẽ phụ thuộc vào. Hỏng ở đây nghĩa là
// hỏng cho mọi bên gọi.

func newSeeded(t *testing.T) (*Module, SeedResult) {
	t.Helper()
	m, err := New(Config{Storage: "memory"})
	if err != nil {
		t.Fatalf("khởi tạo module: %v", err)
	}
	seeded, err := SeedDemo(context.Background(), m, SeedInput{})
	if err != nil {
		t.Fatalf("nạp dữ liệu mẫu: %v", err)
	}
	return m, seeded
}

func TestNewKhoLuuTruKhongHopLe(t *testing.T) {
	if _, err := New(Config{Storage: "mysql"}); err == nil {
		t.Fatal("kho lưu trữ không hợp lệ phải báo lỗi")
	}
	// postgres chưa cài đặt — phải báo rõ, không im lặng trả module rỗng.
	if _, err := New(Config{Storage: "postgres"}); err == nil {
		t.Fatal("postgres chưa cài đặt phải báo lỗi")
	}
}

func TestGetBrandTraDTOKhongPhaiDomain(t *testing.T) {
	m, seeded := newSeeded(t)

	v, err := m.GetBrand(context.Background(), seeded.OwnBrandID)
	if err != nil {
		t.Fatalf("GetBrand: %v", err)
	}
	if v.Name != "Lumière" {
		t.Errorf("tên sai: %q", v.Name)
	}
	if !v.IsOwnBrand {
		t.Error("thương hiệu của nền tảng phải có IsOwnBrand=true")
	}
	if v.ProtectionLevel != "RESTRICTED" {
		t.Errorf("mức bảo vệ sai: %q", v.ProtectionLevel)
	}
}

func TestGetBrandLoiDinhDangVaKhongTonTai(t *testing.T) {
	m, _ := newSeeded(t)
	ctx := context.Background()

	if _, err := m.GetBrand(ctx, "khong-phai-id"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("id sai định dạng phải trả ErrInvalidID, nhận %v", err)
	}
	// ID hợp lệ nhưng không tồn tại → ErrNotFound, phân biệt được với lỗi định dạng.
	if _, err := m.GetBrand(ctx, "brd_01J0ABCDEFGHJKMNPQRSTVWXYZ"); !errors.Is(err, ErrNotFound) {
		t.Errorf("id không tồn tại phải trả ErrNotFound, nhận %v", err)
	}
}

// TestGetBrandsByIDsBoQuaIDHong kiểm tra thiết kế theo lô: một id hỏng
// KHÔNG được làm hỏng cả lời gọi.
//
// Hiển thị 49/50 sản phẩm tốt hơn là cả trang lỗi.
func TestGetBrandsByIDsBoQuaIDHong(t *testing.T) {
	m, seeded := newSeeded(t)

	got, err := m.GetBrandsByIDs(context.Background(), []string{
		seeded.OwnBrandID,
		"rac",
		seeded.ThirdPartyID,
	})
	if err != nil {
		t.Fatalf("GetBrandsByIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("mong 2 thương hiệu, nhận %d", len(got))
	}
	if _, ok := got[seeded.OwnBrandID]; !ok {
		t.Error("thiếu thương hiệu của nền tảng")
	}
}

// TestCanSellerCreateOffer là năng lực CHỐNG HÀNG GIẢ — quan trọng nhất mà
// module marketplace phụ thuộc vào.
func TestCanSellerCreateOffer(t *testing.T) {
	m, seeded := newSeeded(t)
	ctx := context.Background()
	seller := "sel_01J0ABCDEFGHJKMNPQRSTVWXYZ"

	t.Run("thương hiệu mở thì được bán", func(t *testing.T) {
		p, err := m.CanSellerCreateOffer(ctx, seeded.ThirdPartyID, seller)
		if err != nil {
			t.Fatalf("lỗi: %v", err)
		}
		if !p.Allowed {
			t.Errorf("thương hiệu OPEN phải được bán, lý do: %s", p.Reason)
		}
	})

	t.Run("thương hiệu của nền tảng thì KHÔNG", func(t *testing.T) {
		p, err := m.CanSellerCreateOffer(ctx, seeded.OwnBrandID, seller)
		if err != nil {
			t.Fatalf("lỗi: %v", err)
		}
		if p.Allowed {
			t.Error("thương hiệu RESTRICTED của nền tảng KHÔNG được cho seller ngoài bán")
		}
		// Kết quả phải nói RÕ lý do và hành động, không chỉ false — seller
		// cần biết phải làm gì tiếp theo.
		if p.Reason == "" {
			t.Error("thiếu lý do từ chối")
		}
	})

	t.Run("id sai định dạng", func(t *testing.T) {
		if _, err := m.CanSellerCreateOffer(ctx, "rac", seller); !errors.Is(err, ErrInvalidID) {
			t.Errorf("mong ErrInvalidID, nhận %v", err)
		}
	})
}

func TestGetCollectionKemSoTuanConLai(t *testing.T) {
	m, seeded := newSeeded(t)

	v, err := m.GetCollection(context.Background(), seeded.LaunchedColID)
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if !v.IsVisible {
		t.Error("bộ sưu tập đã ra mắt phải hiển thị")
	}
	// Mùa dài 90 ngày → còn khoảng 12-13 tuần. Đây là đầu vào cho quyết
	// định bổ sung hàng, giá trị 0 nghĩa là tính sai.
	if v.WeeksRemaining < 10 || v.WeeksRemaining > 14 {
		t.Errorf("số tuần còn lại bất thường: %d", v.WeeksRemaining)
	}

	// Bộ chưa ra mắt vẫn TRUY VẤN ĐƯỢC qua API nội bộ (module product cần
	// biết để chuẩn bị), nhưng cờ IsVisible phải là false.
	u, err := m.GetCollection(context.Background(), seeded.UnlaunchedColID)
	if err != nil {
		t.Fatalf("GetCollection (chưa ra mắt): %v", err)
	}
	if u.IsVisible {
		t.Error("bộ sưu tập chưa ra mắt KHÔNG được đánh dấu hiển thị")
	}
}

func TestGetCategoryTreeCoCapCon(t *testing.T) {
	m, _ := newSeeded(t)

	tree, err := m.GetCategoryTree(context.Background())
	if err != nil {
		t.Fatalf("GetCategoryTree: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("mong 2 danh mục gốc, nhận %d", len(tree))
	}
	if len(tree[0].Children) == 0 {
		t.Error("danh mục gốc phải kèm cấp con trong cùng một lời gọi")
	}
}

// TestGetSizeChartForCoSoDoThucTe kiểm tra bảng size trả về SỐ ĐO, không
// chỉ ký hiệu S/M/L.
//
// Sai size là nguyên nhân hoàn hàng số một trong thời trang; bảng chỉ có
// S/M/L không giúp khách chọn đúng.
func TestGetSizeChartForCoSoDoThucTe(t *testing.T) {
	m, seeded := newSeeded(t)

	sc, err := m.GetSizeChartFor(context.Background(), seeded.OwnBrandID, "TOP")
	if err != nil {
		t.Fatalf("GetSizeChartFor: %v", err)
	}
	if len(sc.Entries) != 3 {
		t.Fatalf("mong 3 cỡ, nhận %d", len(sc.Entries))
	}
	for _, e := range sc.Entries {
		if len(e.Measurements) == 0 {
			t.Errorf("cỡ %q không có số đo — bảng size vô dụng với khách", e.Size)
		}
		if _, ok := e.Measurements["chest_cm"]; !ok {
			t.Errorf("cỡ %q thiếu chest_cm", e.Size)
		}
	}
}

// TestDTOChiChuaKieuCoBan kiểm tra DTO công khai không mang theo hành vi
// của domain.
//
// BrandView phải là dữ liệu THUẦN: nếu nó chứa domain object, module khác
// sẽ gọi được Brand.Deactivate() và sửa trạng thái ngoài tầm kiểm soát của
// module sở hữu. Việc *kiểu* trả về là *BrandView đã được trình biên dịch
// bảo đảm qua `var _ API = (*Module)(nil)` trong module.go; ở đây kiểm tra
// nội dung DTO đúng như mong đợi.
func TestDTOChiChuaKieuCoBan(t *testing.T) {
	m, seeded := newSeeded(t)

	v, err := m.GetBrand(context.Background(), seeded.OwnBrandID)
	if err != nil {
		t.Fatalf("GetBrand: %v", err)
	}

	// Mọi trường phải là chuỗi/bool — đọc được, không gọi được.
	if v.ID == "" || !strings.HasPrefix(v.ID, BrandIDPrefix+"_") {
		t.Errorf("ID phải là chuỗi có tiền tố %q, nhận %q", BrandIDPrefix, v.ID)
	}
	if v.Status == "" {
		t.Error("Status phải là chuỗi, không phải kiểu enum của domain")
	}
}
