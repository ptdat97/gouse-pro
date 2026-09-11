// Lệnh doisoatpheu dựng lại KHÚC GIỮA của phễu chuyển đổi cho dữ liệu cũ.
//
// # Vì sao cần
//
// `checkout.started` và `checkout.expired` được khai trong eventbus và đặc
// tả trong docs/02-domain/domain-events.md từ lâu — không ai phát, không ai
// nghe. Phễu chỉ có hai đầu:
//
//	add_to_cart    9.668
//	checkout_start     0   ← khúc giữa TRỐNG
//	order.placed   3.207
//
// Trong khi bảng `checkout` có 8.714 phiên, 5.522 trong đó bỏ dở (63,4%).
//
// Từ nay hai event được phát cho mọi phiên MỚI. Lệnh này dọn phần QUÁ KHỨ:
// phiên cũ đã kết thúc từ lâu nên sẽ không bao giờ phát event nữa, và nếu
// không dựng lại thì phễu có một vách đứng ngay tại ngày triển khai — nhìn
// như thể khách đột ngột bắt đầu thanh toán, chứ không phải như thể ta đột
// ngột bắt đầu ĐO.
//
// # Vì sao ghi thẳng vào analytics chứ không phát event
//
// Phát `checkout.started` cho 8.714 phiên đã chết sẽ đánh thức MỌI bên
// nhận của loại event đó — hôm nay chỉ có analytics, nhưng bên nhận thứ hai
// thêm vào tháng sau sẽ nhận một trận lũ event về những phiên không còn
// tồn tại. Event là mệnh lệnh cho tương lai; dựng lại quá khứ thì ghi
// thẳng vào nơi cần dữ liệu.
//
// # Cách dùng
//
//	doisoatpheu        # chỉ BÁO CÁO, không đổi gì
//	doisoatpheu -ghi   # thực sự dựng lại
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func main() {
	if err := chay(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(1)
	}
}

// phienCu là một phiên thanh toán chưa có mặt trong phễu.
type phienCu struct {
	checkoutID string
	cartID     string
	customerID string
	trangThai  string
	soDong     int
	subtotal   int64
	currency   string
	moLuc      time.Time
	ketLuc     *time.Time
}

func chay() error {
	ghi := flag.Bool("ghi", false,
		"THỰC SỰ dựng lại (mặc định chỉ báo cáo)")
	gioiHan := flag.Int("gioihan", 20000, "số phiên xử lý mỗi lượt")
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

	thieu, err := timPhienThieu(ctx, db.Pool(), *gioiHan)
	if err != nil {
		return err
	}

	if len(thieu) == 0 {
		fmt.Println("Không có phiên nào thiếu khỏi phễu.")
		return nil
	}

	var boDo int
	var tienBoDo int64
	for _, p := range thieu {
		if p.trangThai == "EXPIRED" {
			boDo++
			tienBoDo += p.subtotal
		}
	}
	fmt.Printf("Tìm thấy %d phiên chưa có mặt trong phễu.\n", len(thieu))
	fmt.Printf("Trong đó %d phiên BỎ DỞ, tổng %d đ không đo được.\n\n",
		boDo, tienBoDo)

	if !*ghi {
		fmt.Println("CHỈ BÁO CÁO. Chạy lại với -ghi để dựng lại.")
		return nil
	}

	mod, err := analytics.New(analytics.Config{Storage: "postgres", DB: db})
	if err != nil {
		return fmt.Errorf("dựng module analytics: %w", err)
	}

	var daGhi, boQua int
	for _, p := range thieu {
		if err := ghiPhien(ctx, mod, p); err != nil {
			// KHÔNG dừng cả lượt vì một phiên hỏng.
			fmt.Fprintf(os.Stderr, "  bỏ qua %s: %v\n", p.checkoutID, err)
			boQua++
			continue
		}
		daGhi++
	}

	fmt.Printf("Đã dựng lại %d phiên, bỏ qua %d.\n", daGhi, boQua)
	return nil
}

// ghiPhien ghi bước MỞ, và bước HẾT HẠN nếu phiên đã bỏ dở.
//
// EventID là mã phiên, KHÔNG phải mã ngẫu nhiên: chỉ mục chống trùng là
// (event_id, event_name), nên chạy lệnh này lần thứ hai không nhân đôi dữ
// liệu. Hai bước có tên khác nhau nên chúng không chặn nhau.
func ghiPhien(ctx context.Context, mod *analytics.Module, p phienCu) error {
	tien := p.subtotal
	chung := analytics.EventInput{
		Category:    analytics.CategoryBusiness,
		EventID:     p.checkoutID,
		SessionID:   p.cartID,
		CustomerID:  p.customerID,
		SubjectType: "checkout",
		SubjectID:   p.checkoutID,
		Amount:      &tien,
		Currency:    p.currency,
		Properties:  map[string]any{"line_count": p.soDong},
	}

	moi := chung
	moi.Name = analytics.EventCheckoutStart
	moi.OccurredAt = p.moLuc
	if err := mod.TrackEvent(ctx, moi); err != nil {
		return fmt.Errorf("ghi bước mở phiên: %w", err)
	}

	if p.trangThai != "EXPIRED" {
		return nil
	}

	het := chung
	het.Name = analytics.EventCheckoutExpired
	het.OccurredAt = p.moLuc
	if p.ketLuc != nil {
		het.OccurredAt = *p.ketLuc
	}
	if err := mod.TrackEvent(ctx, het); err != nil {
		return fmt.Errorf("ghi bước hết hạn: %w", err)
	}
	return nil
}

// timPhienThieu tìm phiên chưa có bản ghi `checkout_start` nào.
//
// `subtotal` tính từ DÒNG HÀNG chứ không đọc cột tổng của phiên: phiên bỏ
// dở có thể chưa kịp tính phí ship và thuế, nên cột tổng của chúng là 0 —
// và khi đó "bỏ dở bao nhiêu tiền" luôn trả lời 0.
func timPhienThieu(
	ctx context.Context, pool *pgxpool.Pool, gioiHan int,
) ([]phienCu, error) {
	rows, err := pool.Query(ctx, `
		SELECT c.id, c.cart_id, c.customer_id, c.status, c.currency,
		       c.created_at, c.updated_at,
		       count(l.id),
		       coalesce(sum(l.unit_price * l.quantity), 0)
		  FROM checkout c
		  LEFT JOIN checkout_line l ON l.checkout_id = c.id
		 WHERE NOT EXISTS (
		       SELECT 1 FROM event_log e
		        WHERE e.event_id = c.id AND e.event_name = $1)
		 GROUP BY c.id
		 ORDER BY c.created_at
		 LIMIT $2`, analytics.EventCheckoutStart, gioiHan)
	if err != nil {
		return nil, fmt.Errorf("tìm phiên thiếu: %w", err)
	}
	defer rows.Close()

	var out []phienCu
	for rows.Next() {
		var p phienCu
		var ketLuc *time.Time
		if err := rows.Scan(&p.checkoutID, &p.cartID, &p.customerID,
			&p.trangThai, &p.currency, &p.moLuc, &ketLuc,
			&p.soDong, &p.subtotal); err != nil {
			return nil, fmt.Errorf("đọc phiên: %w", err)
		}
		p.ketLuc = ketLuc
		out = append(out, p)
	}
	return out, rows.Err()
}
