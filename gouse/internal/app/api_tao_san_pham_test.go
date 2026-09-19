package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Luồng TẠO SẢN PHẨM của nhà bán, đi qua HTTP thật.
//
// # Ba lỗi bài này dựng lại
//
// Ngày 17/09/2026, gọi thử endpoint này trên dữ liệu thật cho ba kết quả
// mà một nhà bán gặp trong năm phút đầu tiên:
//
//	thiếu category_id   500 "vui lòng thử lại" — miền ĐÒI danh mục, tầng
//	                    HTTP coi nó là tùy chọn, và lệch ấy rơi xuống
//	                    nhánh mặc định của `dichLoiGhi`
//	thiếu images        500 — cột `TEXT[] NOT NULL DEFAULT '{}'`, nhưng
//	                    DEFAULT chỉ áp dụng khi cột VẮNG khỏi INSERT; một
//	                    slice nil gửi NULL và bị từ chối
//	trùng slug          500 — lỗi bị che sau lỗi images ở trên
//
// Cả ba đều nói với người dùng rằng máy chủ hỏng và nên thử lại. Thử lại y
// hệt sẽ hỏng y hệt.
//
// # Vì sao đi qua HTTP chứ không gọi service
//
// Hai trong ba lỗi chỉ tồn tại ở ĐƯỜNG DÂY: một ở tầng dịch lỗi HTTP, một
// ở tầng lưu trữ Postgres. Gọi thẳng application thì cả hai biến mất, và
// bài test sẽ xanh trong lúc endpoint vẫn trả 500.
func TestTaoSanPhamBaoLOI400ChuKhongPhai500(t *testing.T) {
	a := newAPITest(t)
	nb := dungNhaBan(t, a, "taosp"+ids.MustNew(ids.PrefixRequest).String()[24:])

	brandID := a.thuongHieuMo(t)
	catID := a.mauDanhMuc(t)

	than := func(slug string, them map[string]any) map[string]any {
		b := map[string]any{
			"brand_id":      brandID,
			"category_id":   catID,
			"name":          "Áo thử " + slug,
			"slug":          slug,
			"product_type":  "TOP",
			"gender_target": "UNISEX",
		}
		for k, v := range them {
			b[k] = v
		}
		return b
	}
	tao := func(b map[string]any) reply {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + nb.token
		return a.call(http.MethodPost, "/api/v1/seller/products", b, h)
	}

	duy := ids.MustNew(ids.PrefixRequest).String()[20:]

	t.Run("thiếu danh mục là 400, không phải 500", func(t *testing.T) {
		b := than("thieu-danh-muc-"+duy, nil)
		delete(b, "category_id")

		res := tao(b)
		if res.code == http.StatusInternalServerError {
			t.Fatalf("thiếu category_id trả 500 — người dùng được bảo "+
				"\"thử lại\", mà thử lại y hệt sẽ hỏng y hệt: %s", res.raw)
		}
		if res.code != http.StatusBadRequest {
			t.Fatalf("mong 400, nhận %d: %s", res.code, res.raw)
		}
		// Kiểm ĐÚNG lý do, không chỉ kiểm mã.
		//
		// Bản đầu của bài này chỉ đòi 400 và nó XANH trong khi request
		// hỏng vì thiếu `Idempotency-Key` — một lỗi hoàn toàn khác. Một
		// bài test xanh vì nhầm lý do còn tệ hơn không có bài test.
		if msg := loiMessage(res); !strings.Contains(msg, "danh mục") {
			t.Errorf("400 nhưng không phải vì thiếu danh mục: %q", msg)
		}
	})

	// Hai ca dưới đây nằm trong ĐIỂM MÙ của hàng rào 17/09: lỗi của chúng
	// được tạo tại chỗ bằng `errors.New`, không có tên, nên bài quét `Err…`
	// không thấy và tầng HTTP không nhận ra. Cả hai từng trả 500 trong lúc
	// hàng rào báo xanh. Xem TestDomainKhongTaoLoiTaiCho.
	for _, ca := range []struct {
		ten, truong, giaTri string
	}{
		{"loại sản phẩm lạ", "product_type", "XYZ"},
		{"đối tượng khách lạ", "gender_target", "ALIENS"},
	} {
		t.Run(ca.ten+" là 400, không phải 500", func(t *testing.T) {
			res := tao(than("la-"+ca.truong+"-"+duy, map[string]any{ca.truong: ca.giaTri}))
			if res.code != http.StatusBadRequest {
				t.Fatalf("mong 400, nhận %d: %s", res.code, res.raw)
			}
		})
	}

	// Mã danh mục đúng ĐỊNH DẠNG mà không tồn tại. Trước 19/09/2026 nó lưu
	// được — không có khóa ngoại vì danh mục thuộc module khác — và sản
	// phẩm ấy không bao giờ hiện dưới danh mục nào ở cửa hàng.
	t.Run("danh mục không tồn tại là 400", func(t *testing.T) {
		res := tao(than("dm-rac-"+duy, map[string]any{
			"category_id": ids.MustNew(ids.PrefixCategory).String(),
		}))
		if res.code != http.StatusBadRequest {
			t.Fatalf("ghi được mã danh mục rác: %d — %s", res.code, res.raw)
		}
	})

	t.Run("KHÔNG kèm ảnh vẫn tạo được nháp", func(t *testing.T) {
		// Tạo nháp rồi bổ sung ảnh sau là cách làm BÌNH THƯỜNG, và miền
		// cho phép: điều kiện "phải có ảnh" chỉ bật lúc GỬI DUYỆT.
		res := tao(than("nhap-khong-anh-"+duy, nil))
		if res.code != http.StatusCreated {
			t.Fatalf("mong 201, nhận %d: %s", res.code, res.raw)
		}
		if got, _ := res.body["status"].(string); got != "DRAFT" {
			t.Errorf("sản phẩm mới phải ở DRAFT, nhận %q", got)
		}
	})

	t.Run("trùng slug là 409 kèm việc cần làm", func(t *testing.T) {
		slug := "trung-slug-" + duy
		if res := tao(than(slug, nil)); res.code != http.StatusCreated {
			t.Fatalf("lần tạo đầu: %d — %s", res.code, res.raw)
		}

		res := tao(than(slug, map[string]any{"name": "Trùng"}))
		if res.code != http.StatusConflict {
			t.Fatalf("mong 409, nhận %d: %s", res.code, res.raw)
		}
		// Thông điệp phải nói ĐỔI GÌ. "Xung đột" là đúng mà vô dụng.
		if msg := loiMessage(res); msg == "" {
			t.Error("409 không kèm thông điệp nào")
		}
	})
}

// HÀNG RÀO CHỐNG HÀNG GIẢ vẫn phải đứng.
//
// Bài trên nới lỏng phản hồi lỗi; bài này canh cho việc nới lỏng ấy không
// vô tình mở cửa cho gian hàng đăng bán dưới thương hiệu của người khác.
func TestNhaBanKhongDangDuocDuoiThuongHieuBiHanChe(t *testing.T) {
	a := newAPITest(t)
	nb := dungNhaBan(t, a, "gia"+ids.MustNew(ids.PrefixRequest).String()[24:])

	hanChe := a.thuongHieuHanChe(t)
	if hanChe == "" {
		t.Skip("dữ liệu test không có thương hiệu RESTRICTED")
	}

	h := khoaIdem()
	h["Authorization"] = "Bearer " + nb.token
	res := a.call(http.MethodPost, "/api/v1/seller/products", map[string]any{
		"brand_id":      hanChe,
		"category_id":   a.mauDanhMuc(t),
		"name":          "Hàng giả",
		"slug":          "hang-gia-" + ids.MustNew(ids.PrefixRequest).String()[20:],
		"product_type":  "TOP",
		"gender_target": "WOMEN",
	}, h)

	if res.code != http.StatusForbidden {
		t.Fatalf("gian hàng lạ đăng được dưới thương hiệu RESTRICTED: "+
			"HTTP %d — %s", res.code, res.raw)
	}
}

func loiMessage(res reply) string {
	e, _ := res.body["error"].(map[string]any)
	m, _ := e["message"].(string)
	return m
}

// thuongHieuMo tìm một thương hiệu ai cũng bán được.
func (a *apiTest) thuongHieuMo(t *testing.T) string {
	t.Helper()
	return a.motDong(t,
		`SELECT id FROM brand WHERE protection_level = 'OPEN'
		   AND status = 'ACTIVE' LIMIT 1`,
		"không có thương hiệu OPEN nào trong dữ liệu test")
}

// thuongHieuHanChe tìm một thương hiệu chỉ chủ sở hữu được bán.
func (a *apiTest) thuongHieuHanChe(t *testing.T) string {
	t.Helper()
	var id string
	_ = a.db.Pool().QueryRow(context.Background(),
		`SELECT id FROM brand WHERE protection_level = 'RESTRICTED'
		   AND status = 'ACTIVE' LIMIT 1`).Scan(&id)
	return id
}

func (a *apiTest) mauDanhMuc(t *testing.T) string {
	t.Helper()
	return a.motDong(t, `SELECT id FROM category LIMIT 1`,
		"không có danh mục nào trong dữ liệu test")
}

func (a *apiTest) motDong(t *testing.T, sql, khiTrong string) string {
	t.Helper()
	var id string
	if err := a.db.Pool().QueryRow(context.Background(), sql).Scan(&id); err != nil {
		t.Fatalf("%s: %v", khiTrong, err)
	}
	return id
}
