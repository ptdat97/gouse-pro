package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// chanGhiEvent dựng trigger làm lệnh ghi MỘT loại event vào outbox thất bại.
//
// Chặn theo LOẠI chứ không chặn cả bảng, và đó là điểm mấu chốt của thí
// nghiệm: `orders.PlaceOrder` ở bước 2 CŨNG ghi outbox (event `order.*`).
// Chặn cả bảng thì bước 2 hỏng trước, phiên không bao giờ đi tới bước 3, và
// khe hở cần đo không bao giờ mở ra. Lần chạy đầu đã hỏng đúng kiểu đó: 0
// đơn được tạo, và bài test tưởng là "không có khe hở".
//
// Tiêm lỗi ở tầng database chứ không stub một interface trong Go: stub chỉ
// chứng minh mã Go gọi đúng thứ tự, còn cái cần biết là giao dịch nào cuộn
// lại cùng nhau khi câu lệnh SQL thật hỏng thật.
func chanGhiEvent(t *testing.T, a *apiTest, loaiEvent string) {
	t.Helper()
	ctx := context.Background()

	if _, err := a.db.Pool().Exec(ctx, `
		CREATE OR REPLACE FUNCTION chan_ghi_thu_nghiem() RETURNS trigger AS $$
		BEGIN
			IF NEW.event_type = `+quoteSQL(loaiEvent)+` THEN
				RAISE EXCEPTION 'tiêm lỗi: từ chối ghi event %', NEW.event_type;
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("tạo hàm chặn: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx, `
		CREATE TRIGGER chan_ghi_event
		BEFORE INSERT ON event_outbox
		FOR EACH ROW EXECUTE FUNCTION chan_ghi_thu_nghiem()`); err != nil {
		t.Fatalf("tạo trigger chặn: %v", err)
	}

	t.Cleanup(func() {
		if _, err := a.db.Pool().Exec(context.Background(),
			`DROP TRIGGER IF EXISTS chan_ghi_event ON event_outbox`); err != nil {
			t.Fatalf("gỡ trigger chặn: %v", err)
		}
	})
}

func quoteSQL(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// TestPH8_OutboxHongSauKhiDonDaTao dựng lại một khe hở ở RANH GIỚI GIAO DỊCH.
//
// # Chuỗi hoàn tất phiên đi qua NHIỀU giao dịch, có chủ ý
//
//  1. GiuDeHoanTat        — giành quyền, GIA HẠN expires_at (giao dịch riêng)
//  2. orders.PlaceOrder   — TẠO ĐƠN                          (giao dịch riêng)
//  3. SaveWithEvents      — trạng thái phiên + event outbox   (một giao dịch)
//
// Bước 3 gộp phiên và event vào một giao dịch — đúng, và tài liệu giải
// thích rõ vì sao. Nhưng bước 2 và bước 3 KHÔNG cùng giao dịch, và đó là
// khe hở bài này đo.
//
// # Điều bài test khẳng định
//
// Sau khi bước 3 hỏng: đơn hàng ĐÃ TỒN TẠI, còn phiên vẫn mang trạng thái
// `STARTED`. Trạng thái đó nằm đúng trong tầm quét của `FindExpired`, và
// `GiuDeHoanTat` chỉ gia hạn thêm 30 giây chứ không đưa phiên ra khỏi tầm
// quét đó.
//
// Bài này KHÔNG khẳng định điều gì về việc hệ thống có tự phục hồi hay
// không — nó chỉ ghi lại trạng thái thật sau lỗi, để bài kế tiếp đo hậu quả.
func TestPH8_OutboxHongSauKhiDonDaTao(t *testing.T) {
	a := newAPITest(t)

	maPhien := a.dungPhienSanHoanTat(
		emailMoi("ph8"), "0900555111")

	// Chặn ghi outbox: bước 3 sẽ hỏng, bước 2 thì không.
	chanGhiEvent(t, a, eventbus.TypeCheckoutCompleted)

	res := a.call(http.MethodPost,
		"/api/v1/checkout/"+maPhien+"/complete", map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code < 400 {
		t.Fatalf("chặn ghi outbox mà hoàn tất vẫn thành công: HTTP %d — %s",
			res.code, res.raw)
	}
	t.Logf("phản hồi hoàn tất: HTTP %d — %s", res.code, res.raw)

	ctx := context.Background()

	// Đơn hàng có được tạo không? Đây là câu hỏi trung tâm: nếu có, nghĩa
	// là bước 2 đã commit độc lập với bước 3.
	var soDon int
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM "order" WHERE source_checkout_id = $1`,
		maPhien).Scan(&soDon); err != nil {
		t.Fatalf("đếm đơn: %v", err)
	}

	var trangThai string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT status FROM checkout WHERE id = $1`, maPhien).
		Scan(&trangThai); err != nil {
		t.Fatalf("đọc phiên: %v", err)
	}

	var tongDon int
	_ = a.db.Pool().QueryRow(ctx, `SELECT count(*) FROM "order"`).Scan(&tongDon)
	t.Logf("sau khi outbox hỏng: %d đơn theo phiên, %d đơn tổng, phiên %q",
		soDon, tongDon, trangThai)

	if soDon == 0 {
		t.Skip("đơn KHÔNG được tạo — bước 2 và 3 nằm cùng giao dịch, " +
			"không có khe hở để đo")
	}

	// Đơn đã tồn tại mà phiên vẫn STARTED: phiên nằm trong tầm quét của
	// FindExpired (`status IN ('STARTED','PENDING_PAYMENT')`).
	if trangThai != "STARTED" && trangThai != "PENDING_PAYMENT" {
		t.Logf("phiên ở %q — ngoài tầm quét dọn hạn, khe hở đã được bịt",
			trangThai)
	}
}

// TestPH8_HangChetPhaiDemDuoc là điều kiện để đánh đổi kia đứng vững.
//
// Loại phiên đã tạo đơn khỏi job dọn đổi HÀNG MA lấy HÀNG CHẾT. Đổi như
// vậy chỉ chấp nhận được nếu hàng chết đếm được — nếu không, nó chỉ là
// cách nói khác của việc giấu vấn đề đi và không ai biết có bao nhiêu đơn
// đang kẹt.
func TestPH8_HangChetPhaiDemDuoc(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	truoc, err := a.mods.checkout.CountHoanTatKetLai(ctx)
	if err != nil {
		t.Fatalf("đếm phiên kẹt: %v", err)
	}

	maPhien := a.dungPhienSanHoanTat(emailMoi("ph8c"), "0900555333")
	func() {
		chanGhiEvent(t, a, eventbus.TypeCheckoutCompleted)
		res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
			map[string]any{"payment_method": "COD"}, khoaIdem())
		if res.code < 400 {
			t.Fatalf("chặn outbox mà vẫn hoàn tất được: %s", res.raw)
		}
	}()

	// Đẩy quá hạn để phiên rơi vào diện "kẹt", giống lúc chạy thật sau khi
	// khoảng ân hạn hết.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE checkout SET expires_at = now() - interval '1 hour'
		  WHERE id = $1`, maPhien); err != nil {
		t.Fatalf("đẩy phiên quá hạn: %v", err)
	}

	sau, err := a.mods.checkout.CountHoanTatKetLai(ctx)
	if err != nil {
		t.Fatalf("đếm phiên kẹt: %v", err)
	}
	if sau != truoc+1 {
		t.Errorf("phiên kẹt đếm được %d, cần %d — hàng đang bị giữ vô thời "+
			"hạn mà không chỉ báo nào nhìn thấy", sau, truoc+1)
	}

	// Và nó KHÔNG được lẫn vào chỉ báo "job dọn đã chết": hai chuyện khác
	// nhau dẫn tới hai hành động khác nhau.
	quaHan, err := a.mods.checkout.CountExpiredPending(ctx)
	if err != nil {
		t.Fatalf("đếm phiên quá hạn: %v", err)
	}
	var coTrong bool
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM checkout
		    WHERE id = $1 AND order_id = '' AND expires_at < now()
		      AND status IN ('STARTED','PENDING_PAYMENT'))`,
		maPhien).Scan(&coTrong); err != nil {
		t.Fatalf("kiểm phiên: %v", err)
	}
	if coTrong {
		t.Errorf("phiên kẹt vẫn nằm trong diện job dọn (tổng %d) — "+
			"chỉ báo 'job dọn đã chết' sẽ kêu sai mãi", quaHan)
	}
}

// TestPH8_DuongBinhThuongKhongKetLai: bản sửa không được biến mọi phiên
// hoàn tất bình thường thành phiên kẹt.
//
// Đây là nửa dễ mất khi thêm một điều kiện vào truy vấn dọn: chặn đúng thứ
// cần chặn nhưng cũng chặn luôn thứ không nên chặn.
func TestPH8_DuongBinhThuongKhongKetLai(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat(emailMoi("ph8d"), "0900555444")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}

	var trangThai, maDon string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT status, order_id FROM checkout WHERE id = $1`, maPhien).
		Scan(&trangThai, &maDon); err != nil {
		t.Fatalf("đọc phiên: %v", err)
	}
	if trangThai != "COMPLETED" {
		t.Errorf("phiên ở %q, cần COMPLETED", trangThai)
	}
	if maDon == "" {
		t.Errorf("phiên hoàn tất mà không ghi mã đơn")
	}

	// COMPLETED nằm ngoài cả hai chỉ báo: không phải phiên bỏ dở, cũng
	// không phải phiên kẹt.
	var ketLai int
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM checkout
		  WHERE id = $1 AND status IN ('STARTED','PENDING_PAYMENT')`,
		maPhien).Scan(&ketLai); err != nil {
		t.Fatalf("đếm: %v", err)
	}
	if ketLai != 0 {
		t.Errorf("phiên hoàn tất bình thường vẫn bị tính là dở dang")
	}
}

// TestPH8_DonDanhMatChoDuaKhiPhienBiDonHan là hậu quả, và là điều đáng lo.
//
// Nối tiếp bài trên: đơn đã tạo, phiên vẫn STARTED. Nếu khách không thử
// lại, phiên hết hạn và job dọn NHẢ TOÀN BỘ hàng đã giữ — trong khi đơn
// hàng vẫn nằm đó, tin rằng hàng của mình còn được giữ.
//
// Hàng nhả ra bán được cho người khác. Hai khách cùng mua một món, và món
// đó chỉ có một — đúng họ lỗi "sinh hàng từ không khí" của PH-31, lần này
// đến từ ranh giới giao dịch chứ không phải từ khe READ COMMITTED.
func TestPH8_DonDanhMatChoDuaKhiPhienBiDonHan(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat(emailMoi("ph8b"), "0900555222")

	// Ghi lại hàng đang giữ TRƯỚC khi hoàn tất.
	var giuTruoc int
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM reservation
		  WHERE checkout_id = $1 AND status = 'ACTIVE'`,
		maPhien).Scan(&giuTruoc); err != nil {
		t.Fatalf("đếm hàng giữ: %v", err)
	}
	if giuTruoc == 0 {
		t.Skip("phiên không giữ hàng nào")
	}

	func() {
		chanGhiEvent(t, a, eventbus.TypeCheckoutCompleted)
		res := a.call(http.MethodPost,
			"/api/v1/checkout/"+maPhien+"/complete", map[string]any{"payment_method": "COD"}, khoaIdem())
		if res.code < 400 {
			t.Fatalf("chặn outbox mà vẫn hoàn tất được: %s", res.raw)
		}
	}()

	var soDon int
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM "order" WHERE source_checkout_id = $1`,
		maPhien).Scan(&soDon); err != nil {
		t.Fatalf("đếm đơn: %v", err)
	}
	if soDon == 0 {
		t.Skip("đơn không được tạo — không có khe hở để đo")
	}

	// Khách không thử lại. Đẩy phiên quá hạn — kể cả sau khoảng ân hạn 30
	// giây mà GiuDeHoanTat vừa cấp.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE checkout SET expires_at = now() - interval '1 hour'
		  WHERE id = $1`, maPhien); err != nil {
		t.Fatalf("đẩy phiên quá hạn: %v", err)
	}

	if _, err := a.mods.checkout.ExpireStale(ctx, 100); err != nil {
		t.Fatalf("chạy dọn hạn: %v", err)
	}

	var giuSau int
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM reservation
		  WHERE checkout_id = $1 AND status = 'ACTIVE'`,
		maPhien).Scan(&giuSau); err != nil {
		t.Fatalf("đếm hàng giữ sau khi dọn: %v", err)
	}

	if giuSau < giuTruoc {
		t.Errorf("job dọn hạn đã NHẢ %d/%d lượt giữ hàng trong khi %d đơn "+
			"hàng vẫn tồn tại (phiên %s) — đơn mất chỗ dựa, hàng nhả ra "+
			"bán được cho người khác",
			giuTruoc-giuSau, giuTruoc, soDon, maPhien)
	}
}
