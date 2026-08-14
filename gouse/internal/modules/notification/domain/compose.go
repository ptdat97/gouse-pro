package domain

import (
	"fmt"
	"strings"
)

// OrderInfo là dữ liệu để soạn thông báo về một đơn hàng.
//
// TOÀN BỘ đến từ payload event — module này không gọi ngược module nào để
// lấy thêm. Trường nào thiếu thì thông báo thiếu chỗ đó, chứ không đi hỏi.
type OrderInfo struct {
	OrderNumber string
	Recipient   string
	CustomerID  string

	// Items là các món trong đơn, tên đã ĐÓNG BĂNG lúc đặt hàng.
	//
	// Dùng tên đã đóng băng chứ không tra lại: seller đổi tên sản phẩm sau
	// khi khách mua thì email xác nhận vẫn phải khớp với thứ khách đã thấy.
	Items []OrderItem

	Total    int64
	Currency string

	// FONumber và TrackingNumber chỉ có ở thông báo giao hàng.
	FONumber       string
	TrackingNumber string
}

// OrderItem là một món trong thông báo.
type OrderItem struct {
	ProductName string
	Quantity    int
}

// Message là nội dung đã soạn.
type Message struct {
	Subject string
	Body    string
}

// ComposeOrderConfirmed soạn email xác nhận đơn hàng.
//
// Đây là email QUAN TRỌNG NHẤT của hệ thống: nó là bằng chứng đầu tiên
// khách có rằng tiền của họ đã đổi lấy một cam kết.
func ComposeOrderConfirmed(in OrderInfo) Message {
	var b strings.Builder

	fmt.Fprintf(&b, "Cảm ơn bạn đã đặt hàng.\n\n")
	fmt.Fprintf(&b, "Mã đơn hàng: %s\n\n", in.OrderNumber)

	if len(in.Items) > 0 {
		b.WriteString("Các món đã đặt:\n")
		for _, it := range in.Items {
			fmt.Fprintf(&b, "  - %s × %d\n", it.ProductName, it.Quantity)
		}
		b.WriteString("\n")
	}

	if in.Total > 0 {
		fmt.Fprintf(&b, "Tổng tiền: %s\n\n", formatMoney(in.Total, in.Currency))
	}

	// Nói trước về việc đơn có thể được giao thành nhiều gói.
	//
	// Khách đặt ba món từ ba nguồn sẽ nhận ba gói vào ba ngày khác nhau.
	// Không báo trước thì gói đầu tiên đến sẽ làm họ tưởng bị thiếu hàng.
	if len(in.Items) > 1 {
		b.WriteString("Đơn hàng của bạn có thể được giao thành nhiều gói, " +
			"tùy theo nguồn hàng. Chúng tôi sẽ báo bạn mỗi khi có gói được gửi đi.\n\n")
	}

	b.WriteString("Chúng tôi sẽ thông báo khi hàng được gửi đi.\n")

	return Message{
		Subject: fmt.Sprintf("Xác nhận đơn hàng %s", in.OrderNumber),
		Body:    b.String(),
	}
}

// ComposeOrderShipped soạn email báo hàng đã gửi.
func ComposeOrderShipped(in OrderInfo) Message {
	var b strings.Builder

	fmt.Fprintf(&b, "Một phần đơn hàng %s của bạn đã được gửi đi.\n\n", in.OrderNumber)

	if in.TrackingNumber != "" {
		fmt.Fprintf(&b, "Mã vận đơn: %s\n\n", in.TrackingNumber)
	}

	if len(in.Items) > 0 {
		b.WriteString("Các món trong gói này:\n")
		for _, it := range in.Items {
			fmt.Fprintf(&b, "  - %s × %d\n", it.ProductName, it.Quantity)
		}
		b.WriteString("\n")
	}

	b.WriteString("Bạn có thể theo dõi hành trình đơn hàng trong mục Đơn hàng của tôi.\n")

	return Message{
		Subject: fmt.Sprintf("Đơn hàng %s đã được gửi đi", in.OrderNumber),
		Body:    b.String(),
	}
}

// ComposeOrderDelivered soạn email báo đã giao.
func ComposeOrderDelivered(in OrderInfo) Message {
	var b strings.Builder

	fmt.Fprintf(&b, "Đơn hàng %s đã được giao thành công.\n\n", in.OrderNumber)
	b.WriteString("Nếu có vấn đề với sản phẩm, bạn có thể yêu cầu đổi trả " +
		"trong vòng 7 ngày kể từ hôm nay.\n")

	return Message{
		Subject: fmt.Sprintf("Đơn hàng %s đã được giao", in.OrderNumber),
		Body:    b.String(),
	}
}

// ComposeOrderCancelled soạn email báo hủy.
//
// Lý do là BẮT BUỘC trong nội dung: khách nhận thông báo hủy mà không biết
// vì sao sẽ gọi tổng đài, và đó là chi phí lẽ ra tránh được.
func ComposeOrderCancelled(in OrderInfo, reason string) Message {
	var b strings.Builder

	fmt.Fprintf(&b, "Một phần đơn hàng %s của bạn đã bị hủy.\n\n", in.OrderNumber)

	if reason != "" {
		fmt.Fprintf(&b, "Lý do: %s\n\n", reason)
	}

	if len(in.Items) > 0 {
		b.WriteString("Các món bị hủy:\n")
		for _, it := range in.Items {
			fmt.Fprintf(&b, "  - %s × %d\n", it.ProductName, it.Quantity)
		}
		b.WriteString("\n")
	}

	b.WriteString("Số tiền tương ứng sẽ được hoàn lại theo phương thức " +
		"thanh toán ban đầu.\n")

	return Message{
		Subject: fmt.Sprintf("Đơn hàng %s có thay đổi", in.OrderNumber),
		Body:    b.String(),
	}
}

// formatMoney hiển thị số tiền cho người đọc.
//
// Tiền lưu bằng SỐ NGUYÊN đơn vị nhỏ nhất. Với VND, đơn vị nhỏ nhất là
// đồng nên không chia; với USD phải chia 100. Xử lý đúng ở đây vì đây là
// chỗ DUY NHẤT trong module hiển thị tiền cho người dùng.
func formatMoney(amount int64, currency string) string {
	if currency == "USD" {
		return fmt.Sprintf("%d.%02d USD", amount/100, amount%100)
	}
	return fmt.Sprintf("%s %s", groupThousands(amount), orDefault(currency, "VND"))
}

// groupThousands chèn dấu chấm phân cách hàng nghìn: 1250000 → 1.250.000.
func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + groupThousands(-n)
	}
	if len(s) <= 3 {
		return s
	}

	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
