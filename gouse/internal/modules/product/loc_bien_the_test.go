package product_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/modules/product/infrastructure/inmemory"
	productpg "github.com/fashion-commerce/platform/internal/modules/product/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// Lọc theo BIẾN THỂ phải cho cùng kết quả ở CẢ HAI kho.
//
// # Vì sao bài này tồn tại
//
// `MODULES_STORAGE` mặc định là `memory` khi phát triển. Tới 24/09/2026 bản
// in-memory BỎ QUA hẳn `Sizes` và `ColorFamilies`, nên mọi lượt chạy ở máy
// lập trình viên đều trả về TOÀN BỘ danh mục bất kể lọc gì:
//
//	color=GREEN   →  3 sản phẩm   (in-memory, sai)
//	color=GREEN   →  0 sản phẩm   (postgres, đúng)
//
// Không lỗi, không log. Một người dựng bộ lọc màu ở cửa hàng sẽ thấy trang
// chạy "được" — có sản phẩm hiện ra — và không bao giờ biết bộ lọc không
// làm gì cả.
//
// Chính hàm `matches` của bản in-memory mở đầu bằng lời cảnh báo rằng nó
// phải GIỐNG bản PostgreSQL, "nếu không test in-memory sẽ xanh cho hành vi
// mà production không có". Hai bộ lọc ngay bên dưới lời cảnh báo ấy là chỗ
// nó bị vi phạm.
//
// # Vì sao so HAI kho chứ không kiểm từng kho
//
// Hai bài test riêng cho hai kho vẫn xanh khi một kho quên mất một bộ lọc —
// bài của nó chỉ đơn giản là không có. Bài này hỏi một câu khác: hai cài
// đặt có TRẢ LỜI GIỐNG NHAU không.

// hangThu là một sản phẩm thử cùng các biến thể của nó.
type hangThu struct {
	ten     string
	bienThe []bienTheThu
}

type bienTheThu struct {
	mau, size string
}

var danhMucThu = []hangThu{
	{"Áo sơ mi linen", []bienTheThu{{"Trắng", "M"}, {"Xanh navy", "L"}}},
	{"Áo thun cổ tròn", []bienTheThu{{"Đen", "M"}}},
	{"Túi tote canvas", []bienTheThu{{"Be", "Free"}}},

	// Biến thể nhiều size cùng một màu: bản PostgreSQL dùng EXISTS chứ
	// không JOIN, nên sản phẩm phải hiện ra ĐÚNG MỘT LẦN.
	{"Quần jean ống suông", []bienTheThu{
		{"Xanh denim", "29"},
		{"Xanh denim", "30"},
		{"Xanh denim", "31"},
	}},
}

// KHÔNG có ca biến thể ARCHIVED ở đây, dù truy vấn PostgreSQL loại chúng
// bằng `v.status <> 'ARCHIVED'`.
//
// Lý do: tới 24/09/2026 KHÔNG mã nào đặt biến thể sang trạng thái ấy —
// `NewVariant` luôn tạo ACTIVE và không có hàm nào chuyển. Điều kiện SQL
// đang canh một trạng thái chưa tới được.
//
// Dựng nó bằng `RestoreVariant` sẽ là một bài test cho tình huống ứng dụng
// không tạo ra nổi, và một bài như thế nói dối về phạm vi đang được bảo
// vệ. Khi nào lưu trữ biến thể thành việc làm được, thêm ca vào đây — và
// nhớ rằng bản in-memory đã sẵn sàng (xem `domain.Filter.KhopBienThe`).

func dungHang(t *testing.T, h hangThu) *domain.Product {
	t.Helper()
	now := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

	p, err := domain.NewProduct(domain.NewProductParams{
		BrandID:             ids.MustNew(ids.PrefixBrand),
		CategoryID:          ids.MustNew(ids.PrefixCategory),
		SizeChartID:         ids.MustNew(ids.PrefixSizeChart),
		Name:                h.ten,
		Slug:                slugThu(h.ten),
		Description:         "Mô tả",
		MaterialComposition: "100% cotton",
		ProductType:         domain.ProductTypeTop,
		GenderTarget:        domain.GenderUnisex,
		Images:              []string{"https://cdn.example.com/1.jpg"},
		Now:                 now,
	})
	if err != nil {
		t.Fatalf("NewProduct(%s): %v", h.ten, err)
	}

	for _, b := range h.bienThe {
		v, err := domain.NewVariant(domain.NewVariantParams{
			Attributes: map[string]string{"color": b.mau, "size": b.size},
			Now:        now,
		})
		if err != nil {
			t.Fatalf("NewVariant: %v", err)
		}
		if err := p.AddVariant(v, now); err != nil {
			t.Fatalf("AddVariant: %v", err)
		}
	}
	return p
}

func slugThu(ten string) string {
	s := strings.ToLower(ten)
	s = strings.NewReplacer(
		"á", "a", "à", "a", "ả", "a", "ã", "a", "ạ", "a", "â", "a", "ă", "a",
		"é", "e", "è", "e", "ê", "e", "ể", "e",
		"í", "i", "ì", "i", "ó", "o", "ò", "o", "ổ", "o", "ô", "o", "ơ", "o",
		"ú", "u", "ù", "u", "ư", "u", "ừ", "u",
		"đ", "d", "ị", "i", "ậ", "a", "ắ", "a", "ạ", "a", " ", "-",
	).Replace(s)
	return s + "-" + strings.ToLower(ids.MustNew(ids.PrefixProduct).String()[20:])
}

func tenSapXep(ps []*domain.Product) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name())
	}
	sort.Strings(out)
	return out
}

func TestHaiKhoLocBienTheGiongNhau(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)

	if _, err := db.Pool().Exec(ctx,
		"TRUNCATE product, variant, sku CASCADE"); err != nil {
		t.Fatalf("dọn danh mục: %v", err)
	}

	pg := productpg.NewProductStore(db.Pool())
	mem := inmemory.NewProductStore()

	for _, h := range danhMucThu {
		p := dungHang(t, h)
		if err := pg.Save(ctx, p); err != nil {
			t.Fatalf("ghi postgres: %v", err)
		}
		if err := mem.Save(ctx, p); err != nil {
			t.Fatalf("ghi in-memory: %v", err)
		}
	}

	bai := []struct {
		ten string
		f   domain.Filter
	}{
		{"không lọc", domain.Filter{}},
		{"một nhóm màu", domain.Filter{ColorFamilies: []string{"WHITE"}}},
		{"nhóm màu khác", domain.Filter{ColorFamilies: []string{"BLACK"}}},
		{"nhiều nhóm màu", domain.Filter{ColorFamilies: []string{"BLACK", "BEIGE"}}},
		{"nhóm màu không có hàng", domain.Filter{ColorFamilies: []string{"GREEN"}}},

		// Nhóm màu là hằng số hệ thống nhưng client có thể gửi chữ thường.
		{"nhóm màu chữ thường", domain.Filter{ColorFamilies: []string{"blue"}}},

		// Một sản phẩm có BA biến thể cùng màu: EXISTS phải cho nó hiện
		// đúng một lần, không phải ba.
		{"nhiều biến thể cùng màu", domain.Filter{ColorFamilies: []string{"BLUE"}}},

		{"một size", domain.Filter{Sizes: []string{"M"}}},
		{"size chữ hoa", domain.Filter{Sizes: []string{"FREE"}}},
		{"size không có", domain.Filter{Sizes: []string{"XXL"}}},

		// Hai bộ lọc cùng lúc phải GIAO nhau, không phải hợp.
		{"size và màu cùng lúc",
			domain.Filter{Sizes: []string{"M"}, ColorFamilies: []string{"BLACK"}}},
		{"size và màu không cùng sản phẩm",
			domain.Filter{Sizes: []string{"Free"}, ColorFamilies: []string{"BLACK"}}},
	}

	for _, b := range bai {
		t.Run(b.ten, func(t *testing.T) {
			raPG, err := pg.List(ctx, b.f)
			if err != nil {
				t.Fatalf("postgres.List: %v", err)
			}
			raMem, err := mem.List(ctx, b.f)
			if err != nil {
				t.Fatalf("inmemory.List: %v", err)
			}

			tenPG, tenMem := tenSapXep(raPG), tenSapXep(raMem)
			if strings.Join(tenPG, "|") != strings.Join(tenMem, "|") {
				t.Fatalf("hai kho trả khác nhau\n  postgres : %v\n  in-memory: %v",
					tenPG, tenMem)
			}
			t.Logf("cả hai: %v", tenPG)
		})
	}
}
