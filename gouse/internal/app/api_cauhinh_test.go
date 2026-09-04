package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// khoiPhucCauHinh trả tham số về mặc định khi bài test kết thúc.
//
// Tham số vận hành là trạng thái TOÀN CỤC của database test: đổi rồi bỏ đó
// làm mọi bài chạy sau nhìn thấy giá trị lạ. Bài `TestHieuSuat...` khẳng
// định SLA là 48 và đã đỏ đúng vì lý do này.
func (a *apiTest) khoiPhucCauHinh(t *testing.T, khoa string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := a.db.Pool().Exec(context.Background(),
			`DELETE FROM ops_config WHERE khoa = $1`, khoa); err != nil {
			t.Fatalf("khôi phục cấu hình %s: %v", khoa, err)
		}
		if err := a.mods.opsConfig.NapLai(context.Background()); err != nil {
			t.Fatalf("nạp lại cấu hình: %v", err)
		}
	})
}

func (a *apiTest) datCauHinh(
	t *testing.T, tok, khoa string, giaTri float64, lyDo string,
) reply {
	t.Helper()
	a.khoiPhucCauHinh(t, khoa)
	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	return a.call(http.MethodPut, "/api/v1/admin/config/"+khoa,
		map[string]any{"value": giaTri, "reason": lyDo}, h)
}

// TestCauHinhDoiNgayLapTuc — đường cơ bản, và là toàn bộ lý do tồn tại.
//
// Trước tính năng này, đổi hạn giao hàng phải build lại và triển khai lại.
func TestCauHinhDoiNgayLapTuc(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)
	sellerID := a.baoDamCoDonThucHien(t)
	tokNB := a.taoTokenNhaBan(t, sellerID)

	// Trước: SLA mặc định.
	r := a.xemHieuSuat(t, tokNB, "")
	if got, _ := r.body["shipping_sla_hours"].(float64); got != 48 {
		t.Fatalf("SLA ban đầu = %v, cần 48", got)
	}

	res := a.datCauHinh(t, tok, opsconfig.KeySLAGiaoHang, 24,
		"siết hạn giao hàng theo cam kết dịch vụ quý 4 đã thống nhất")
	if res.code != http.StatusOK {
		t.Fatalf("đổi cấu hình: HTTP %d — %s", res.code, res.raw)
	}
	if got, _ := res.body["previous_value"].(float64); got != 48 {
		t.Errorf("previous_value = %v, cần 48 — không có giá trị CŨ thì "+
			"vết kiểm toán không trả lời được câu hỏi 'đổi từ bao nhiêu'", got)
	}

	// Sau: endpoint hiệu suất phải dùng NGAY con số mới, không cần khởi
	// động lại.
	r = a.xemHieuSuat(t, tokNB, "")
	if got, _ := r.body["shipping_sla_hours"].(float64); got != 24 {
		t.Errorf("SLA sau khi đổi = %v, cần 24 — cấu hình không tới được "+
			"chỗ dùng nó: %s", got, r.raw)
	}
}

// TestCauHinhGhiVetKemGiaTriCu.
//
// Đổi tham số vận hành ảnh hưởng tới người NGOÀI công ty: hạ hạn giao làm
// hàng loạt gian hàng đột ngột bị chấm là giao trễ, và điểm đó ảnh hưởng
// tới việc họ thắng buy box. Không có vết thì không giải thích được khi
// nhà bán khiếu nại.
func TestCauHinhGhiVetKemGiaTriCu(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)
	ctx := context.Background()

	dem := func() int {
		var n int
		_ = a.db.Pool().QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE action = 'ops_config.set'`).
			Scan(&n)
		return n
	}
	truoc := dem()

	res := a.datCauHinh(t, tok, opsconfig.KeyMauToiThieu, 25,
		"nâng cỡ mẫu tối thiểu để giảm khiếu nại của gian hàng mới mở")
	if res.code != http.StatusOK {
		t.Fatalf("đổi cấu hình: HTTP %d — %s", res.code, res.raw)
	}

	if sau := dem(); sau != truoc+1 {
		t.Fatalf("số vết = %d, cần %d", sau, truoc+1)
	}

	// Vết phải mang CẢ giá trị cũ và mới.
	var meta string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT metadata::text FROM audit_log
		  WHERE action = 'ops_config.set' ORDER BY occurred_at DESC LIMIT 1`).
		Scan(&meta); err != nil {
		t.Fatalf("đọc vết: %v", err)
	}
	for _, can := range []string{"gia_tri_cu", "gia_tri_moi"} {
		if !contains(meta, can) {
			t.Errorf("vết thiếu %q: %s — 'đổi thành 25' không trả lời được "+
				"câu hỏi quan trọng nhất khi điều tra: đổi từ bao nhiêu",
				can, meta)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestCauHinhDoiLyDo: đổi tham số là thao tác nhạy cảm.
func TestCauHinhDoiLyDo(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	for _, lyDo := range []string{"", "sửa", "test test test test"} {
		res := a.datCauHinh(t, tok, opsconfig.KeyNguongHuyDon, 0.05, lyDo)
		if res.code != http.StatusBadRequest {
			t.Errorf("lý do %q được chấp nhận: HTTP %d — %s",
				lyDo, res.code, res.raw)
		}
	}
}

// TestCauHinhChanGiaTriNgoaiBien.
//
// Một tham số không có biên là một tham số ai đó sẽ đặt bằng 0 và làm sập
// một thứ ở xa. Cỡ mẫu 0 nghĩa là chấm mọi gian hàng dù chỉ có một đơn.
func TestCauHinhChanGiaTriNgoaiBien(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	xau := []struct {
		khoa   string
		giaTri float64
	}{
		{opsconfig.KeyMauToiThieu, 0},      // dưới Min
		{opsconfig.KeyMauToiThieu, 2.5},    // không nguyên
		{opsconfig.KeyNguongHuyDon, 1.5},   // tỷ lệ > 1
		{opsconfig.KeyNguongHuyDon, -0.1},  // âm
		{opsconfig.KeySLAGiaoHang, 0},      // hạn giao 0 giờ
		{opsconfig.KeySLAGiaoHang, 100000}, // hơn 11 năm
	}
	for _, x := range xau {
		res := a.datCauHinh(t, tok, x.khoa, x.giaTri,
			"thử giá trị ngoài biên trong bài kiểm thử tự động")
		if res.code != http.StatusBadRequest {
			t.Errorf("%s = %v được chấp nhận: HTTP %d — %s",
				x.khoa, x.giaTri, res.code, res.raw)
		}
	}
}

// TestCauHinhChanKhoaLa là hàng rào chính của cơ chế này.
//
// Sổ đăng ký ĐÓNG: chỉ khóa khai trong mã mới tồn tại. Không có đường nào
// thêm tham số mới từ giao diện — thêm tham số là việc của người viết mã,
// có review, vì mỗi tham số mới là một câu hỏi "sửa được lúc chạy có an
// toàn không" mà một cái form không trả lời được.
func TestCauHinhChanKhoaLa(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	for _, khoa := range []string{
		"audit.min_reason_len",       // kiểm soát đúng đắn, KHÔNG được sửa
		"identity.max_failed_logins", // kiểm soát an ninh
		"fulfillment.khong_ton_tai",
	} {
		res := a.datCauHinh(t, tok, khoa, 1,
			"thử đặt một khóa không có trong sổ đăng ký")
		if res.code != http.StatusNotFound {
			t.Errorf("khóa lạ %q được chấp nhận: HTTP %d — %s",
				khoa, res.code, res.raw)
		}
	}
}

// TestCauHinhChiSoDangKyMoiHienRa: giao diện phải nêu HỆ QUẢ, không chỉ
// con số.
//
// Người đổi con số hiếm khi là người viết đoạn mã đọc nó, và "48" không tự
// nói rằng hạ nó xuống sẽ làm hàng loạt gian hàng đột ngột bị chấm trễ.
func TestCauHinhChiSoDangKyMoiHienRa(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	res := a.call(http.MethodGet, "/api/v1/admin/config", nil,
		map[string]string{"Authorization": "Bearer " + tok})
	if res.code != http.StatusOK {
		t.Fatalf("đọc cấu hình: HTTP %d — %s", res.code, res.raw)
	}

	ds, _ := res.body["data"].([]any)
	if len(ds) != len(opsconfig.MoiThamSo()) {
		t.Fatalf("trả %d tham số, sổ đăng ký có %d",
			len(ds), len(opsconfig.MoiThamSo()))
	}

	for _, x := range ds {
		m, _ := x.(map[string]any)
		khoa, _ := m["key"].(string)
		for _, truong := range []string{"description", "impact"} {
			if v, _ := m[truong].(string); len(v) < 20 {
				t.Errorf("%s thiếu %s (%q) — một con số không kèm hệ quả "+
					"thì người đổi không biết mình đang đổi gì", khoa, truong, v)
			}
		}
		for _, truong := range []string{"min", "max", "default"} {
			if _, co := m[truong]; !co {
				t.Errorf("%s thiếu %s", khoa, truong)
			}
		}
	}
}
