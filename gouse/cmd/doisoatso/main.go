// Lệnh doisoatso ĐỐI CHIẾU sổ cái với trạng thái đơn, và ĐẢO bút toán sai.
//
// # Vì sao cần một lệnh riêng
//
// Trước ADR-0018, bút toán doanh thu ghi NỢ `PLATFORM_CASH` ngay lúc khách
// hoàn tất phiên thanh toán. Đo được lúc phát hiện:
//
//	PENDING_PAYMENT  3110 bút toán  PLATFORM_CASH ghi NỢ 1.132.273.000 đ
//	PAID                1 bút toán                             390.000 đ
//
// Sổ cái khẳng định nền tảng cầm 1,13 tỷ đồng chưa hề tới. Sổ cái BẤT BIẾN
// (ADR-0008) nên không xóa được — chỉ ghi bút toán ĐẢO.
//
// # Vì sao là lệnh, không phải một câu SQL chạy tay
//
// Một câu UPDATE tay đi vòng qua mọi ràng buộc mà domain cưỡng chế: cân
// bằng nợ-có, một bút toán chỉ đảo một lần, lý do bắt buộc. Và nó không để
// lại vết. Lệnh này đi qua đúng đường mà mọi bút toán khác đi.
//
// Quan trọng hơn: quy trình đảo bút toán phải TỒN TẠI trước khi hệ thống
// chạy thật. Lúc đó thì không còn cách dọn nào khác.
//
// # Cách dùng
//
//	doisoatso                 # chỉ BÁO CÁO, không đổi gì
//	doisoatso -dao -lydo="…"  # thực sự ghi bút toán đảo
//
// Mặc định là CHỈ BÁO CÁO. Một công cụ sửa sổ cái mà chạy nhầm không có
// đường lùi, nên nó phải được gọi tên rõ ràng mới đổi dữ liệu.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func main() {
	if err := chay(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(1)
	}
}

// butToanSai là một bút toán ghi tiền mặt cho đơn chưa thu được tiền.
type butToanSai struct {
	entryID   string
	orderID   string
	trangThai string
	soTien    int64
}

func chay() error {
	dao := flag.Bool("dao", false,
		"THỰC SỰ ghi bút toán đảo (mặc định chỉ báo cáo)")
	lyDo := flag.String("lydo", "",
		"lý do đảo — BẮT BUỘC khi dùng -dao")
	gioiHan := flag.Int("gioihan", 500, "số bút toán xử lý mỗi lượt")
	flag.Parse()

	if *dao && strings.TrimSpace(*lyDo) == "" {
		return fmt.Errorf("-dao BẮT BUỘC đi kèm -lydo: một khoản tiền biến " +
			"mất khỏi sổ mà không ai giải thích được là đúng thứ kiểm toán " +
			"tồn tại để chặn")
	}

	dsn := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("thiếu DATABASE_URL")
	}

	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{DSN: dsn})
	if err != nil {
		return fmt.Errorf("mở database: %w", err)
	}
	defer db.Close()

	sai, err := timButToanSai(ctx, db.Pool(), *gioiHan)
	if err != nil {
		return err
	}

	if len(sai) == 0 {
		fmt.Println("Không có bút toán nào ghi TIỀN MẶT cho đơn chưa thu được tiền.")
		return nil
	}

	var tong int64
	for _, b := range sai {
		tong += b.soTien
	}
	fmt.Printf("Tìm thấy %d bút toán ghi NỢ PLATFORM_CASH cho đơn chưa trả tiền.\n", len(sai))
	fmt.Printf("Tổng: %d đ\n\n", tong)

	for i, b := range sai {
		if i >= 5 {
			fmt.Printf("  … và %d bút toán nữa\n", len(sai)-5)
			break
		}
		fmt.Printf("  %s  đơn %s  %-16s %12d đ\n",
			b.entryID, b.orderID, b.trangThai, b.soTien)
	}

	if !*dao {
		fmt.Println("\nCHỈ BÁO CÁO. Chạy lại với -dao -lydo=\"…\" để ghi bút toán đảo.")
		return nil
	}

	mod, err := payment.New(payment.Config{Storage: "postgres", DB: db})
	if err != nil {
		return fmt.Errorf("dựng module payment: %w", err)
	}

	var daDao, boQua int
	for _, b := range sai {
		if _, err := mod.DaoButToan(ctx, b.entryID, *lyDo, "doisoatso"); err != nil {
			// KHÔNG dừng cả lượt vì một bút toán hỏng: phần còn lại vẫn
			// đảo được, và dừng giữa chừng để lại sổ ở trạng thái nửa vời
			// khó đọc hơn hẳn.
			fmt.Fprintf(os.Stderr, "  bỏ qua %s: %v\n", b.entryID, err)
			boQua++
			continue
		}
		daDao++
	}

	fmt.Printf("\nĐã đảo %d bút toán, bỏ qua %d.\n", daDao, boQua)
	return nil
}

// timButToanSai tìm bút toán ghi NỢ tiền mặt cho đơn CHƯA thu được tiền.
//
// Điều kiện "chưa thu được tiền" đọc từ trạng thái ĐƠN, không từ sổ cái:
// sổ cái chính là thứ đang sai, nên dùng nó làm căn cứ là đi vòng tròn.
//
// Bỏ qua bút toán ĐÃ đảo: `reverses_entry_id` trỏ về chúng.
func timButToanSai(
	ctx context.Context, pool *pgxpool.Pool, gioiHan int,
) ([]butToanSai, error) {
	rows, err := pool.Query(ctx, `
		SELECT e.id, o.id, o.status,
		       COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'PLATFORM_CASH'
		             AND l.direction = 'DEBIT'), 0)
		  FROM ledger_entry e
		  JOIN ledger_line  l ON l.entry_id = e.id
		  JOIN "order"      o ON o.id = e.reference_id
		 WHERE e.entry_type = 'ORDER_REVENUE'
		   AND o.status = 'PENDING_PAYMENT'
		   AND NOT EXISTS (
		       SELECT 1 FROM ledger_entry d
		        WHERE d.reverses_entry_id = e.id)
		 GROUP BY e.id, o.id, o.status
		HAVING COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'PLATFORM_CASH'
		             AND l.direction = 'DEBIT'), 0) > 0
		 ORDER BY e.created_at
		 LIMIT $1`, gioiHan)
	if err != nil {
		return nil, fmt.Errorf("tìm bút toán sai: %w", err)
	}
	defer rows.Close()

	var out []butToanSai
	for rows.Next() {
		var b butToanSai
		if err := rows.Scan(&b.entryID, &b.orderID, &b.trangThai, &b.soTien); err != nil {
			return nil, fmt.Errorf("đọc bút toán: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
