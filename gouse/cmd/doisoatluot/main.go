// Lệnh doisoatluot ĐỐI CHIẾU lượt dùng mã giảm giá với các đơn đã đặt.
//
// # Vì sao cần
//
// `RecordUsage` có đủ ba tầng từ lâu, kèm cả phần khó nhất: ghi lượt rồi
// cộng dồn NGUYÊN TỬ, idempotent theo đơn. Nhưng KHÔNG ai gọi nó từ ngoài
// module, nên mọi đơn dùng mã trước hôm nay đều không để lại lượt nào.
//
// Hệ quả trên dữ liệu cũ: bộ đếm `used_count` và `used_budget` thấp hơn
// sự thật. Chừng nào mã chưa đặt trần thì không ai thấy gì; ngày đặt trần
// đầu tiên, mã sẽ sống lâu hơn đúng số đơn đã lỡ.
//
// Từ nay bên nhận `promotion.ghi_luot_dung_khi_hoan_tat` ghi lượt cho mọi
// đơn mới. Lệnh này chỉ để dọn phần QUÁ KHỨ — event của những đơn đó đã
// phát xong nên bên nhận không bao giờ thấy chúng nữa.
//
// # Vì sao là lệnh, không phải job chạy nền
//
// Đây là việc MỘT LẦN. Một job chạy mãi để dọn một khoảng quá khứ hữu hạn
// sẽ quét vô ích mỗi phút, và tệ hơn: nó che mất việc đường ghi lượt bình
// thường có đang chạy hay không.
//
// # Cách dùng
//
//	doisoatluot        # chỉ BÁO CÁO, không đổi gì
//	doisoatluot -ghi   # thực sự ghi lượt còn thiếu
//
// Mặc định CHỈ BÁO CÁO: ghi nhầm một lượt là lấy mất quyền dùng mã của
// một khách có thật.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/promotion"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func main() {
	if err := chay(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(1)
	}
}

// luotThieu là một đơn đã dùng mã nhưng không có lượt nào được ghi.
type luotThieu struct {
	orderID    string
	customerID string
	code       string
	discount   int64
	currency   string
	trangThai  string
}

func chay() error {
	ghi := flag.Bool("ghi", false,
		"THỰC SỰ ghi lượt còn thiếu (mặc định chỉ báo cáo)")
	gioiHan := flag.Int("gioihan", 500, "số đơn xử lý mỗi lượt")
	flag.Parse()

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

	// Rà chiều NGƯỢC trước: lượt còn hiệu lực của đơn đã hủy.
	//
	// Chiều này chỉ báo cáo. Giải phóng nhầm thì mã được dùng thêm một
	// lượt ngoài dự tính — rẻ hơn ghi nhầm, nhưng vẫn là quyết định của
	// người, vì một đơn hủy rồi đặt lại là chuyện có thật.
	if err := raSoatDonDaHuy(ctx, db.Pool()); err != nil {
		return err
	}

	thieu, err := timLuotThieu(ctx, db.Pool(), *gioiHan)
	if err != nil {
		return err
	}

	if len(thieu) == 0 {
		fmt.Println("Không có đơn nào dùng mã mà thiếu lượt.")
		return nil
	}

	var tong int64
	for _, t := range thieu {
		tong += t.discount
	}
	fmt.Printf("Tìm thấy %d đơn đã dùng mã nhưng KHÔNG có lượt nào được ghi.\n",
		len(thieu))
	fmt.Printf("Tổng tiền đã giảm mà ngân sách khuyến mãi chưa tính: %d đ\n\n", tong)

	for i, t := range thieu {
		if i >= 5 {
			fmt.Printf("  … và %d đơn nữa\n", len(thieu)-5)
			break
		}
		fmt.Printf("  %s  mã %-16s %-16s %10d đ\n",
			t.orderID, t.code, t.trangThai, t.discount)
	}

	if !*ghi {
		fmt.Println("\nCHỈ BÁO CÁO. Chạy lại với -ghi để ghi lượt còn thiếu.")
		return nil
	}

	mod, err := promotion.New(promotion.Config{Storage: "postgres", DB: db})
	if err != nil {
		return fmt.Errorf("dựng module promotion: %w", err)
	}

	var daGhi, boQua int
	for _, t := range thieu {
		// Đi qua module, KHÔNG chèn thẳng SQL: bộ đếm `used_count` và
		// `used_budget` phải tăng cùng lúc với hàng lượt, và chỉ đường
		// này làm đúng cả hai.
		err := mod.RecordUsage(ctx, promotion.RecordUsageRequest{
			Code:       t.code,
			CustomerID: t.customerID,
			OrderID:    t.orderID,
			Discount:   t.discount,
			Currency:   t.currency,
		})
		if err != nil {
			// KHÔNG dừng cả lượt vì một đơn hỏng — mã đã bị xóa hoặc đổi
			// tên là chuyện thường với dữ liệu cũ.
			fmt.Fprintf(os.Stderr, "  bỏ qua %s (mã %s): %v\n",
				t.orderID, t.code, err)
			boQua++
			continue
		}
		daGhi++
	}

	fmt.Printf("\nĐã ghi %d lượt, bỏ qua %d.\n", daGhi, boQua)
	if boQua > 0 {
		return errors.New("có đơn không ghi được lượt — xem log ở trên")
	}
	return nil
}

// timLuotThieu tìm đơn CÓ giảm giá mà không có hàng nào trong coupon_usage.
//
// Mã đọc từ PHIÊN THANH TOÁN, không từ đơn: đơn chỉ giữ SỐ TIỀN giảm, còn
// mã nào tạo ra số đó thì chỉ phiên biết.
//
// Bỏ qua đơn ĐÃ HỦY: ghi lượt cho chúng rồi lại phải giải phóng ngay là
// hai lần sai thay vì không lần nào.
func timLuotThieu(
	ctx context.Context, pool *pgxpool.Pool, gioiHan int,
) ([]luotThieu, error) {
	rows, err := pool.Query(ctx, `
		SELECT o.id, o.customer_id, c.coupon_code, o.discount_amount,
		       o.currency, o.status
		  FROM "order" o
		  JOIN checkout c ON c.order_id = o.id
		 WHERE o.discount_amount > 0
		   AND c.coupon_code <> ''
		   AND o.status <> 'CANCELLED'
		   AND NOT EXISTS (
		       SELECT 1 FROM coupon_usage u WHERE u.order_id = o.id)
		 ORDER BY o.created_at
		 LIMIT $1`, gioiHan)
	if err != nil {
		return nil, fmt.Errorf("tìm lượt thiếu: %w", err)
	}
	defer rows.Close()

	var out []luotThieu
	for rows.Next() {
		var t luotThieu
		if err := rows.Scan(&t.orderID, &t.customerID, &t.code,
			&t.discount, &t.currency, &t.trangThai); err != nil {
			return nil, fmt.Errorf("đọc đơn: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// raSoatDonDaHuy báo các lượt còn hiệu lực thuộc về đơn đã hủy.
//
// Mỗi hàng ở đây là một khách đang bị giữ mất quyền dùng mã cho một đơn
// không còn tồn tại.
func raSoatDonDaHuy(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT u.order_id, u.discount_amount
		  FROM coupon_usage u
		  JOIN "order" o ON o.id = u.order_id
		 WHERE o.status = 'CANCELLED'
		   AND u.released_at IS NULL
		 ORDER BY u.used_at
		 LIMIT 20`)
	if err != nil {
		return fmt.Errorf("rà đơn đã hủy: %w", err)
	}
	defer rows.Close()

	var n int
	for rows.Next() {
		var id string
		var tien int64
		if err := rows.Scan(&id, &tien); err != nil {
			return fmt.Errorf("đọc lượt: %w", err)
		}
		if n == 0 {
			fmt.Println("Lượt còn hiệu lực của đơn ĐÃ HỦY " +
				"(khách đang mất quyền dùng mã):")
		}
		fmt.Printf("  %s  %10d đ\n", id, tien)
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n > 0 {
		fmt.Printf("→ %d lượt. Giải phóng từng cái sau khi xác nhận đơn "+
			"không được đặt lại.\n\n", n)
	}
	return nil
}
