package payment_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// Hàng rào cân bằng sổ cái ở tầng DATABASE (migration 000056).
//
// # Vì sao test này đi VÒNG QUA mã Go
//
// Miền đã từ chối bút toán lệch, và đã có test cho việc đó. Bài này kiểm
// một thứ khác: chuyện gì xảy ra khi có người KHÔNG đi qua miền.
//
// Đó không phải giả thiết xa vời. Dự án này đã có bốn lệnh `cmd/doisoat*`
// nối thẳng vào database, đã có một lần sửa dữ liệu bằng migration, và sẽ
// có thêm. Một hàng rào chỉ nằm ở tầng ứng dụng thì không che được những
// đường đó — đúng như `CHECK (… >= 0)` của tồn kho phải nằm ở database chứ
// không nằm trong Go.
//
// Vì thế mọi bài dưới đây ghi bằng SQL thô.
func moSoCai(t *testing.T) *pgxpool.Pool {
	t.Helper()
	db := testdb.Open(t)
	p := db.Pool()

	// TRUNCATE chứ không DELETE: trigger bất biến chặn DELETE từng dòng.
	// CASCADE vì `ledger_line` tham chiếu `ledger_entry`.
	if _, err := p.Exec(context.Background(),
		"TRUNCATE ledger_entry, ledger_line CASCADE"); err != nil {
		t.Fatalf("dọn sổ cái: %v", err)
	}
	return p
}

// dong là một dòng sổ cái viết gọn cho bài test.
type dong struct {
	taiKhoan string
	huong    string
	soTien   int64
	donVi    string
}

// ghiButToan ghi một bút toán kèm các dòng của nó trong MỘT giao dịch.
//
// Thứ tự — bút toán trước, dòng sau — CỐ TÌNH giống hệt
// `payment/infrastructure/postgres/store.go`. Chính thứ tự này là lý do
// hàng rào phải là constraint trigger HOÃN: một trigger thường sẽ chạy lúc
// bảng dòng còn rỗng và từ chối cả bút toán đúng.
func ghiButToan(t *testing.T, p *pgxpool.Pool, id string, dongs ...dong) error {
	t.Helper()
	ctx := context.Background()

	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatalf("mở giao dịch: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entry (id, entry_type, reference_type, reference_id,
			description, idempotency_key, created_by, created_at)
		VALUES ($1,'ADJUSTMENT','test',$1,'',$1,'test',now())`, id); err != nil {
		return err
	}
	for _, d := range dongs {
		// Tài khoản PHẢI TRẢ bắt buộc có chủ sở hữu
		// (`ledger_line_payable_needs_owner`, migration 000007): một
		// `SELLER_PAYABLE` không biết của seller nào thì không đối soát
		// được. Điền sẵn ở đây để bảng dữ liệu của từng bài chỉ còn
		// những thứ bài ấy thật sự nói về.
		chuSoHuu := ""
		if strings.HasSuffix(d.taiKhoan, "_PAYABLE") {
			chuSoHuu = "sel_TEST0000000000000000000001"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_line (entry_id, account_type, account_owner_id,
				direction, amount, currency, description)
			VALUES ($1,$2,$3,$4,$5,$6,'')`,
			id, d.taiKhoan, chuSoHuu, d.huong, d.soTien, d.donVi); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// maSo dựng một id hợp lệ: `led_` + 26 ký tự, theo CHECK của bảng.
func maSo(i int) string { return fmt.Sprintf("led_TEST%022d", i) }

func TestButToanCanBangThiGhiDuoc(t *testing.T) {
	p := moSoCai(t)

	err := ghiButToan(t, p, maSo(1),
		dong{"PLATFORM_CASH", "DEBIT", 1000, "VND"},
		dong{"PLATFORM_REVENUE", "CREDIT", 1000, "VND"},
	)
	if err != nil {
		t.Fatalf("bút toán cân bằng bị từ chối: %v", err)
	}
}

// Ca này là lý do migration 000056 tồn tại.
func TestButToanLechBiDatabaseTuChoi(t *testing.T) {
	p := moSoCai(t)

	err := ghiButToan(t, p, maSo(2),
		dong{"PLATFORM_CASH", "DEBIT", 1000, "VND"},
		dong{"PLATFORM_REVENUE", "CREDIT", 999, "VND"},
	)
	if err == nil {
		t.Fatal("database CHO QUA một bút toán lệch nợ-có")
	}

	// Thông điệp phải nêu ĐỦ để người trực sự cố tài chính hành động ngay:
	// bút toán nào, đơn vị nào, lệch bao nhiêu. "Ràng buộc bị vi phạm" thì
	// đúng mà vô dụng lúc 3 giờ sáng.
	for _, can := range []string{maSo(2), "VND", "1000", "999"} {
		if !strings.Contains(err.Error(), can) {
			t.Errorf("thông điệp thiếu %q: %v", can, err)
		}
	}
}

// Bút toán RỖNG phải kiểm RIÊNG.
//
// Σ của tập rỗng bằng 0 ở cả hai vế, nên một phép so cân bằng đơn thuần
// CHO QUA bút toán không có dòng nào. Nó không sai số học — nó chỉ không
// nói gì, và nó chiếm mất khóa idempotency, nên lần ghi lại ĐÚNG sẽ bị từ
// chối là trùng.
func TestButToanKhongCoDongNaoBiTuChoi(t *testing.T) {
	p := moSoCai(t)

	err := ghiButToan(t, p, maSo(3))
	if err == nil {
		t.Fatal("database CHO QUA một bút toán không có dòng nào")
	}
	if !strings.Contains(err.Error(), "không có dòng nào") {
		t.Errorf("thông điệp không nói rõ vì sao: %v", err)
	}
}

// LẪN đơn vị tiền: cộng cột `amount` thì "cân", mà bút toán vô nghĩa.
//
// Đây là lý do hàng rào gom theo `currency` thay vì cộng tất. Một phép
// kiểm cộng tất sẽ cho 100 JPY NỢ đối 100 VND CÓ đi qua.
func TestButToanLanDonViTienBiTuChoi(t *testing.T) {
	p := moSoCai(t)

	err := ghiButToan(t, p, maSo(4),
		dong{"PLATFORM_CASH", "DEBIT", 1000, "VND"},
		dong{"PLATFORM_REVENUE", "CREDIT", 1000, "JPY"},
	)
	if err == nil {
		t.Fatal("database CHO QUA bút toán lẫn hai đơn vị tiền")
	}
}

// Bút toán NHIỀU DÒNG mỗi bên vẫn phải đi qua được.
//
// Bài này giữ hàng rào khỏi bị siết quá tay: chuỗi hoàn tiền thật chia một
// vế thành hai — đảo hoa hồng nền tảng và đảo số dư nhà bán (returns/
// application/service.go). Một hàng rào đòi đúng một dòng mỗi bên sẽ chặn
// đúng luồng quan trọng nhất của module.
func TestNhieuDongMoiBenVanCanBang(t *testing.T) {
	p := moSoCai(t)

	err := ghiButToan(t, p, maSo(5),
		dong{"PLATFORM_CASH", "DEBIT", 1000, "VND"},
		dong{"PLATFORM_REVENUE", "CREDIT", 300, "VND"},
		dong{"SELLER_PAYABLE", "CREDIT", 700, "VND"},
	)
	if err != nil {
		t.Fatalf("bút toán ba dòng cân bằng bị từ chối: %v", err)
	}
}

// HAI đơn vị tiền trong một bút toán, mỗi đơn vị TỰ cân — vẫn cho qua.
//
// Hàng rào kiểm cân bằng theo TỪNG đơn vị, nên nó không cấm một bút toán
// chạm hai đơn vị; nó chỉ đòi mỗi đơn vị tự cân. Ghi rõ ranh giới ấy ở đây
// để lần sau không ai siết nhầm thành "một bút toán một đơn vị tiền" —
// một quyết định khác hẳn, và nếu muốn thì phải quyết có chủ ý.
func TestHaiDonViTienMoiBenTuCanThiChoQua(t *testing.T) {
	p := moSoCai(t)

	err := ghiButToan(t, p, maSo(6),
		dong{"PLATFORM_CASH", "DEBIT", 1000, "VND"},
		dong{"PLATFORM_REVENUE", "CREDIT", 1000, "VND"},
		dong{"PLATFORM_CASH", "DEBIT", 50, "JPY"},
		dong{"PLATFORM_REVENUE", "CREDIT", 50, "JPY"},
	)
	if err != nil {
		t.Fatalf("mỗi đơn vị tự cân mà vẫn bị từ chối: %v", err)
	}
}
