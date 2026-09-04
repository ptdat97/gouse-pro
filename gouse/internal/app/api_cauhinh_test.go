package app

import (
	"context"
	"net/http"
	"sync"
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

// TestCauHinhGhiVetHongThiKhongDoi.
//
// Ghi vết và ghi giá trị nằm trong CÙNG một giao dịch. Nếu chúng tách rời,
// ghi vết xong mà ghi giá trị hỏng sẽ để lại một dòng nhật ký nói tham số
// ĐÃ đổi — trong khi nó chưa đổi. Nhật ký kiểm toán nói dối còn tệ hơn
// không có nhật ký, vì người điều tra tin vào nó.
//
// # Tiêm lỗi ở ĐÚNG bước ghi
//
// Bản đầu của bài này đổi tên cả bảng `ops_config`. Không dùng được: lệnh
// hỏng đầu tiên là lệnh ĐỌC giá trị cũ, chạy TRƯỚC cả lúc ghi vết — nên
// hai cách làm (ghi vết trong và ngoài giao dịch) cho ra cùng kết quả, và
// bài test không phân biệt được. Phá để kiểm đã chỉ ra điều đó.
//
// Trigger chỉ chặn INSERT: đọc vẫn chạy, ghi vết vẫn chạy, chỉ lệnh ghi
// giá trị hỏng — đúng khoảng mà bài này cần đo.
func TestCauHinhGhiVetHongThiKhongDoi(t *testing.T) {
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

	if _, err := a.db.Pool().Exec(ctx, `
		CREATE OR REPLACE FUNCTION chan_ghi_cauhinh() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'tiêm lỗi: từ chối ghi tham số';
		END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("tạo hàm chặn: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx, `
		CREATE TRIGGER chan_ghi_cauhinh BEFORE INSERT ON ops_config
		FOR EACH ROW EXECUTE FUNCTION chan_ghi_cauhinh()`); err != nil {
		t.Fatalf("tạo trigger: %v", err)
	}
	t.Cleanup(func() {
		if _, err := a.db.Pool().Exec(context.Background(),
			`DROP TRIGGER IF EXISTS chan_ghi_cauhinh ON ops_config`); err != nil {
			t.Fatalf("gỡ trigger: %v", err)
		}
	})

	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res := a.call(http.MethodPut,
		"/api/v1/admin/config/"+opsconfig.KeyNguongHuyDon,
		map[string]any{
			"value":  0.07,
			"reason": "nới ngưỡng hủy theo thỏa thuận với nhóm vận hành quý này",
		}, h)
	if res.code < 400 {
		t.Fatalf("ghi giá trị hỏng mà vẫn báo thành công: %s", res.raw)
	}

	if sau := dem(); sau != truoc {
		t.Errorf("ghi giá trị HỎNG mà vẫn còn %d vết mới — nhật ký nói "+
			"tham số đã đổi trong khi nó chưa đổi", sau-truoc)
	}
}

// TestCauHinhHaiNguoiDoiCungLucVetVanDUNG.
//
// Giá trị CŨ đi vào vết kiểm toán. Hai quản trị viên đổi cùng một tham số
// cùng lúc mà không khóa thì CẢ HAI cùng đọc giá trị cũ giống nhau, và một
// trong hai dòng nhật ký ghi sai điểm xuất phát — "đổi từ 48 thành 36"
// trong khi thực tế nó đi từ 24.
//
// Bài này khẳng định hai dòng vết tạo thành một CHUỖI: giá trị cũ của lượt
// sau phải bằng giá trị mới của lượt trước.
func TestCauHinhHaiNguoiDoiCungLucVetVanDung(t *testing.T) {
	a := newAPITest(t)
	tok, actorID := a.taoTaiKhoanVaiTroCoID(t, identity.RoleAdmin)
	ctx := context.Background()
	a.khoiPhucCauHinh(t, opsconfig.KeySLAGiaoHang)

	const macDinh = 48.0
	moi := []float64{24, 36}

	var xong sync.WaitGroup
	for _, v := range moi {
		xong.Add(1)
		go func(v float64) {
			defer xong.Done()
			h := khoaIdem()
			h["Authorization"] = "Bearer " + tok
			a.call(http.MethodPut,
				"/api/v1/admin/config/"+opsconfig.KeySLAGiaoHang,
				map[string]any{
					"value":  v,
					"reason": "đổi hạn giao trong bài kiểm thử hai người cùng sửa",
				}, h)
		}(v)
	}
	xong.Wait()

	rows, err := a.db.Pool().Query(ctx, `
		SELECT (metadata->>'gia_tri_cu')::float8,
		       (metadata->>'gia_tri_moi')::float8
		  FROM audit_log
		 WHERE action = 'ops_config.set'
		   AND resource_id = $1
		   -- Lọc theo NGƯỜI THỰC HIỆN của riêng bài này: nhật ký là bảng
		   -- dùng chung, và bài khác cũng đổi đúng tham số này.
		   AND actor_id = $2
		 ORDER BY occurred_at`, opsconfig.KeySLAGiaoHang, actorID)
	if err != nil {
		t.Fatalf("đọc vết: %v", err)
	}
	defer rows.Close()

	var cu, mo []float64
	for rows.Next() {
		var c, m float64
		if err := rows.Scan(&c, &m); err != nil {
			t.Fatalf("đọc dòng vết: %v", err)
		}
		cu = append(cu, c)
		mo = append(mo, m)
	}
	if len(cu) != 2 {
		t.Fatalf("có %d vết, cần 2", len(cu))
	}

	// Khẳng định theo TÍNH CHẤT CHUỖI, không theo thứ tự thời gian.
	//
	// `occurred_at` mặc định là `now()`, mà trong PostgreSQL đó là thời
	// điểm BẮT ĐẦU giao dịch — không phải lúc ghi. Khóa tuần tự hóa phần
	// THÂN giao dịch, nên giao dịch thứ hai có thể bắt đầu trước khi giao
	// dịch thứ nhất xong, và sắp theo cột đó cho ra thứ tự sai.
	//
	// Tính chất đúng và không phụ thuộc thứ tự: MỘT vết đi từ mặc định,
	// vết còn lại đi từ giá trị mà vết kia vừa đặt.
	var tuMacDinh, tuVetKia int
	for i := range cu {
		if cu[i] == macDinh {
			tuMacDinh++
			continue
		}
		for j := range mo {
			if j != i && cu[i] == mo[j] {
				tuVetKia++
			}
		}
	}
	if tuMacDinh != 1 || tuVetKia != 1 {
		t.Errorf("hai vết KHÔNG tạo thành chuỗi: cũ=%v mới=%v\n"+
			"đúng ra một vết đi từ %v và vết kia đi từ giá trị vết đầu "+
			"vừa đặt. Cả hai cùng đọc một điểm xuất phát nghĩa là không "+
			"có khóa, và nhật ký ghi sai lịch sử.", cu, mo, macDinh)
	}
}

// TestCauHinhDocKhongKhoa: đọc tham số phải an toàn khi chạy song song.
//
// Đường đọc chạy trên MỌI request tính hiệu suất, nên nó phải chịu được
// đọc song song với ghi. Bài này chạy dưới `-race`.
func TestCauHinhDocKhongKhoa(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)
	store := a.mods.opsConfig
	if store == nil {
		t.Skip("chưa nối cấu hình vận hành")
	}

	var xong sync.WaitGroup
	dung := make(chan struct{})

	// Nhiều goroutine ĐỌC liên tục.
	for i := 0; i < 8; i++ {
		xong.Add(1)
		go func() {
			defer xong.Done()
			for {
				select {
				case <-dung:
					return
				default:
					_ = store.Doc(opsconfig.KeySLAGiaoHang)
					_ = store.DocSoNguyen(opsconfig.KeyMauToiThieu)
				}
			}
		}()
	}

	// Trong lúc đó, GHI qua đường HTTP thật.
	for i := 0; i < 3; i++ {
		res := a.datCauHinh(t, tok, opsconfig.KeySLAGiaoHang, float64(24+i),
			"đổi hạn giao trong bài kiểm thử đọc ghi song song")
		if res.code != http.StatusOK {
			t.Errorf("đổi lần %d: HTTP %d — %s", i, res.code, res.raw)
		}
	}

	close(dung)
	xong.Wait()
}
