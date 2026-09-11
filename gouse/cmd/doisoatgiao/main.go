// Lệnh doisoatgiao điền PHƯƠNG THỨC GIAO còn thiếu của đơn thực hiện.
//
// # Vì sao cần
//
// Trường `shipping_method` của đơn thực hiện được khai trong domain, có
// cột trong database, nằm trong câu `UPDATE`, được đọc lại khi truy vấn —
// và câu `INSERT` bỏ sót nó. Không đường nào gán nó lúc tạo, nên nó rỗng
// trên CẢ 3.207 đơn.
//
// Hệ quả không dừng ở quá khứ: 3.183 đơn trong số đó CHƯA bàn giao. Khi
// chúng được bàn giao, `payment` tra giá trả hãng vận chuyển theo đúng
// trường rỗng đó và không ghi bút toán nào — im lặng, vì "chưa khai giá"
// và "không biết phương thức" cùng dẫn tới giá 0. Ngày giao dự kiến báo
// cho khách cũng tính từ nó.
//
// # Vì sao KHÔNG dựng lại ngày giao dự kiến
//
// Ngày ấy tính từ lúc BÀN GIAO. Với đơn đã giao xong, dựng lại một "ngày
// dự kiến" trong quá khứ là bịa ra một lời hứa chưa từng được đưa ra. Với
// đơn chưa bàn giao, nó sẽ được tính đúng lúc bàn giao thật.
//
// # Cách dùng
//
//	doisoatgiao        # chỉ BÁO CÁO, không đổi gì
//	doisoatgiao -ghi   # thực sự điền
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fashion-commerce/platform/internal/platform/database"
)

func main() {
	if err := chay(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(1)
	}
}

func chay() error {
	ghi := flag.Bool("ghi", false, "THỰC SỰ điền (mặc định chỉ báo cáo)")
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

	// Nguồn sự thật là PHIÊN THANH TOÁN: khách chọn phương thức ở đó, và
	// phí đã thu được tính theo đúng lựa chọn ấy. Suy ngược từ phí đã thu
	// là đoán — hai phương thức có thể ra cùng số tiền sau khuyến mãi.
	const truyVan = `
		SELECT f.id, c.shipping_method, f.shipped_at IS NOT NULL
		  FROM fulfillment_order f
		  JOIN checkout c ON c.order_id = f.order_id
		 WHERE f.shipping_method = '' AND c.shipping_method <> ''`

	rows, err := db.Pool().Query(ctx, truyVan)
	if err != nil {
		return fmt.Errorf("tìm đơn thiếu phương thức giao: %w", err)
	}

	type can struct {
		id         string
		phuongThuc string
	}
	var ds []can
	var daBanGiao int
	for rows.Next() {
		var c can
		var xong bool
		if err := rows.Scan(&c.id, &c.phuongThuc, &xong); err != nil {
			rows.Close()
			return fmt.Errorf("đọc dòng: %w", err)
		}
		if xong {
			daBanGiao++
		}
		ds = append(ds, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(ds) == 0 {
		fmt.Println("Không có đơn thực hiện nào thiếu phương thức giao.")
		return nil
	}

	fmt.Printf("Tìm thấy %d đơn thực hiện thiếu phương thức giao.\n", len(ds))
	fmt.Printf("Trong đó %d đơn ĐÃ bàn giao (bút toán chi phí hãng của chúng "+
		"đã lỡ), %d đơn CHƯA — số này còn kịp.\n\n", daBanGiao, len(ds)-daBanGiao)

	if !*ghi {
		fmt.Println("CHỈ BÁO CÁO. Chạy lại với -ghi để điền.")
		return nil
	}

	// KHÔNG tăng `version`: đây là sửa dữ liệu bị bỏ sót, không phải một
	// bước chuyển trạng thái. Tăng version sẽ làm mọi tiến trình đang giữ
	// bản đọc cũ trong bộ nhớ thất bại vì xung đột khóa lạc quan — với
	// một thay đổi mà chúng không quan tâm.
	tag, err := db.Pool().Exec(ctx, `
		UPDATE fulfillment_order f
		   SET shipping_method = c.shipping_method
		  FROM checkout c
		 WHERE c.order_id = f.order_id
		   AND f.shipping_method = '' AND c.shipping_method <> ''`)
	if err != nil {
		return fmt.Errorf("điền phương thức giao: %w", err)
	}

	fmt.Printf("Đã điền %d đơn.\n", tag.RowsAffected())
	return nil
}
