package main

// intentionallyConstant là những cột CHỈ CÓ MỘT GIÁ TRỊ và điều đó ĐÚNG.
//
// # Vì sao cần sổ này, không phải một danh sách bỏ qua
//
// Phép quét trả về hai loại cột trông giống hệt nhau: cột hằng số có chủ ý,
// và cột không ai ghi. Phân biệt chúng là phán đoán nghiệp vụ, không phải
// phép đo — nên nó phải được một người viết ra kèm lý do, một lần, rồi máy
// giữ.
//
// Khóa là `bảng.cột`. Lý do phải trả lời được: *điều gì khiến cột này KHÔNG
// BAO GIỜ nhận giá trị thứ hai?* Một dòng "chưa dùng" không đạt — "chưa
// dùng" thuộc sổ dưới.
var intentionallyConstant = map[string]string{
	// ---------------------------------------------------- một đồng tiền
	//
	// Sàn hoạt động ở Việt Nam và giá niêm yết bằng VND. Cột `currency` tồn
	// tại vì `money.Money` luôn mang đơn vị — bỏ nó đi là mở đường cho một
	// con số tiền không biết mình là tiền gì. Xem internal/kernel/money.
	"event_log.currency":         "sàn chỉ giao dịch VND; money.Money luôn mang đơn vị",
	"ledger_line.currency":       "sổ cái một đơn vị; bút toán trộn đơn vị bị domain chặn",
	"cart.currency":              "giỏ một đơn vị; ErrMixedCurrency chặn trộn",
	"cart_item.currency":         "theo giỏ",
	"checkout.currency":          "theo giỏ",
	"checkout_line.currency":     "theo giỏ",
	"order.currency":             "đóng băng từ phiên thanh toán",
	"order_line.currency":        "đóng băng từ dòng phiên",
	"fulfillment_order.currency": "theo đơn",
	"metric_snapshot.currency":   "chỉ số tiền đều VND",

	// ------------------------------------------------- chỉ giao nội địa
	"checkout.ship_country_code":          "chỉ giao trong nước; mã quốc gia là VN theo thiết kế",
	"order.ship_country_code":             "theo phiên thanh toán",
	"fulfillment_order.ship_country_code": "theo đơn",

	// --------------------------------------- cột của ĐƯỜNG THẤT BẠI
	//
	// Rỗng/NULL ở đây nghĩa là KHÔNG CÓ GÌ HỎNG. Chúng nhận giá trị chỉ khi
	// một việc thất bại, nên một database khỏe mạnh phải thấy chúng rỗng.
	// Nếu một ngày chúng có nhiều giá trị, đó là tin xấu — không phải tin
	// "cột đã được nối dây".
	"event_outbox.dead_lettered_at":    "chỉ khác NULL khi event vào hàng chết",
	"notification_log.error":           "chỉ khác rỗng khi gửi thư thất bại",
	"fulfillment_order.failure_reason": "chỉ khác rỗng khi hãng báo giao thất bại",
	"fulfillment_order.cancel_reason":  "chỉ khác rỗng khi đơn thực hiện bị hủy",
	"fulfillment_order.cancelled_at":   "chỉ khác NULL khi bị hủy",
	"order_line.cancelled_at":          "chỉ khác NULL khi dòng bị hủy",
	"order.cancellation_reason":        "chỉ khác rỗng khi đơn bị hủy",
	"order_line.status":                "ACTIVE là trạng thái của dòng chưa bị hủy",
	"cart_item.availability":           "AVAILABLE là trạng thái của món còn bán được",
	"reservation.extensions":           "0 là số lần gia hạn của giữ chỗ chưa ai gia hạn",
	"checkout.extended_times":          "0 là số lần gia hạn của phiên chưa ai gia hạn",

	// -------------------------------- mốc thời gian của giai đoạn SAU
	//
	// Khác nhóm trên: đây là đường THÀNH CÔNG, nhưng ở xa cuối vòng đời nên
	// phần lớn hàng chưa tới. Một giá trị duy nhất ở đây là dấu hiệu dữ liệu
	// được gieo một lượt, không phải cột không ai ghi — kiểm bằng cách đếm
	// hàng KHÁC NULL thay vì đếm giá trị phân biệt.
	"order.paid_at":                  "chỉ đơn đã thu tiền có mốc này",
	"order.delivered_at":             "chỉ đơn đã giao có mốc này",
	"order.completed_at":             "chỉ đơn đã hoàn tất có mốc này",
	"fulfillment_order.delivered_at": "chỉ đơn thực hiện đã giao có mốc này",
	"fulfillment_order.completed_at": "chỉ đơn thực hiện đã hoàn tất có mốc này",

	// ------------------------------------------- tham số của offer
	"cart_item.min_order_quantity": "mọi offer hôm nay đặt tối thiểu 1; cột để offer khai khác",
}

// knownEmpty là những cột KHÔNG AI GHI, kèm điều nó làm hỏng.
//
// # Sổ này CHỈ ĐƯỢC RÚT NGẮN
//
// Đây là backlog, không phải danh sách bỏ qua. Mục 8 của
// docs/10-roadmap/backlog.md ghi 14 lần cùng một hình dạng — một trường có
// trong hợp đồng, có cột, có phương thức domain, và không dòng mã nào gán
// giá trị cho nó. Sổ này là chỗ những lần CHƯA sửa nằm chờ, để chúng không
// nằm im.
//
// Lý do phải nói HỆ QUẢ, không nói hiện trạng: "rỗng" là điều phép đo đã
// biết; điều cần người viết là *cái gì không làm được vì nó rỗng*.
var knownEmpty = map[string]string{
	// -------------------- ĐÃ SỬA MÃ, CHỜ DỮ LIỆU ĐUỔI KỊP (30/10/2026)
	//
	// Hai dòng dưới KHÁC phần còn lại của sổ: bên gọi đã có rồi. Chúng ở
	// đây vì công cụ này đo DỮ LIỆU, và dữ liệu đã ghi không đổi khi mã
	// đổi — 16.061 hàng outbox cũ vẫn rỗng causation, 153 hàng chỉ số cũ
	// vẫn mang DAY.
	//
	// KHÔNG xóa ngay: xóa bây giờ làm `make deadcol` đỏ với lý do SAI
	// ("phải QUYẾT" — đã quyết rồi). Cứ để, và khi tiến trình chạy bản
	// mới ghi được giá trị thứ hai, chính công cụ này sẽ đòi xóa chúng
	// bằng lớp vi phạm thứ hai: "SỔ còn dòng ... mà cột ấy NAY ĐÃ CÓ
	// nhiều giá trị — xóa đi".
	//
	// Nói cách khác: dòng sổ ở đây là một PHIẾU CHỜ XÁC NHẬN, và hàng rào
	// tự đòi lại phiếu. Không cần ai nhớ.
	"event_outbox.causation_id": "dữ liệu CŨ không xếp được thành CÂY: " +
		"không trả lời được 'event nào sinh ra event này' cho mọi event " +
		"ghi trước 30/10/2026. `dispatcher` nay truyền event cha xuống " +
		"ngữ cảnh handler — xem `withCausation`",

	"metric_snapshot.granularity": "dữ liệu CŨ chỉ có DAY: không xem lại " +
		"được đột biến TRONG NGÀY của giai đoạn trước 30/10/2026, và báo " +
		"cáo tháng của giai đoạn ấy phải cộng 30 bản ghi ngày lúc đọc. " +
		"`cmd/worker` nay có bên gọi cho HOUR và MONTH — xem " +
		"`metricWindows` và `monthWindows`",

	// ----------------------------------------------------------------

	"event_log.ip_hash": "không chặn được lạm dụng theo IP, và không điều " +
		"tra được một chuỗi hành vi đáng ngờ đến từ đâu",
	"event_log.user_agent": "không phân biệt được lượt của người với lượt " +
		"của bot khi đọc phễu chuyển đổi",

	"demand_signal.product_id": "tín hiệu nhu cầu biết SKU mà không biết " +
		"sản phẩm, nên không gộp được theo sản phẩm — đúng câu hỏi mà " +
		"gợi ý nhập bổ sung (Phase 3) cần",
	"demand_signal.category_id": "không gộp được nhu cầu theo danh mục",

	"fulfillment_order.stock_location_id": "không biết kiện hàng xuất từ " +
		"kho nào, nên không tính được chi phí theo kho và không đối soát " +
		"được tồn kho theo điểm xuất",

	"metric_snapshot.dimension":       "chỉ số chỉ có bản TỔNG, không tách được theo chiều",
	"metric_snapshot.dimension_value": "cùng cột trên",
	// ---------------------------- chuỗi quy công creator (Phase 2)
	//
	// P3-81/84 nối được chuỗi tới `order_line`, nhưng dữ liệu phát triển
	// chưa có lượt mua nào đi qua liên kết creator, nên cột vẫn rỗng. Đây
	// là ca cần đo lại trên dữ liệu THẬT trước khi kết luận.
	"cart_item.source_creator_id":      "chưa có lượt mua nào qua liên kết creator; đo lại khi Phase 2 chạy",
	"cart_item.source_content_id":      "cùng cột trên",
	"checkout_line.source_creator_id":  "cùng cột trên",
	"checkout_line.source_content_id":  "cùng cột trên",
	"order_line.attributed_creator_id": "cùng cột trên",
	"order_line.attributed_content_id": "cùng cột trên",

	"checkout.discount_allocations": "chưa phiên nào áp mã do nhà bán chịu, " +
		"nên nhánh phân bổ chi phí giảm giá chưa chạy trên dữ liệu thật",
	"checkout.discount_cost_bearer": "cùng cột trên",

	"notification_log.category": "chỉ thư GIAO DỊCH được gửi; thư MARKETING " +
		"chưa có đường ra, nên `Category.RequiresConsent()` chưa bao giờ " +
		"được gọi thật — xem P3-54",
	"notification_log.channel": "chỉ kênh EMAIL được nối; SMS khai trong " +
		"enum mà không có bên gửi",

	"fulfillment_order.fulfillment_type": "mọi kiện đều do NHÀ BÁN thực hiện; " +
		"đường nền tảng thực hiện chưa có lượt nào",
	"fulfillment_order.shipping_method": "mọi kiện đều STANDARD; EXPRESS có " +
		"trong biểu phí mà chưa đơn nào chọn",
	"fulfillment_order.awaiting_payment": "mọi đơn trong dữ liệu phát triển " +
		"đã mở khóa; nhánh KHÓA chờ tiền chỉ thấy ở test",

	// ------------------------------------- địa chỉ hóa đơn (7 cột)
	//
	// Một nhóm, một lý do: không giao diện nào thu địa chỉ hóa đơn riêng.
	// Chúng có từ migration 000020 cho hóa đơn VAT — thứ chưa xây.
	"order.bill_recipient_name": "không giao diện nào thu địa chỉ hóa đơn riêng",
	"order.bill_phone":          "cùng nhóm trên",
	"order.bill_street":         "cùng nhóm trên",
	"order.bill_ward":           "cùng nhóm trên",
	"order.bill_district":       "cùng nhóm trên",
	"order.bill_province":       "cùng nhóm trên",
	"order.bill_country_code":   "cùng nhóm trên",
}
