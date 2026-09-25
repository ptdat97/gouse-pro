package main

// Sổ ghi CẶP hình dạng phản hồi: schema đặc tả ⇄ struct Go.
//
// # Vì sao ghi tay
//
// Một phép ghép tự động theo ĐỘ KHỚP tên trường đã được thử ngày
// 25/09/2026. Nó tìm đúng phần lớn cặp, và đưa ra những cặp SAI hẳn:
//
//	BrandRef  ↔ thuongHieuChoBanJSON   khớp 4 trường, hai thứ khác nhau
//	Cart      ↔ checkoutJSON           khớp 5, giỏ hàng ≠ phiên thanh toán
//	Shipment  ↔ sellerFOJSON           khớp 6, lô hàng ≠ đơn thực hiện
//
// Cảnh báo giả là thứ làm người ta tắt hẳn phép kiểm. Nên cặp phải tường
// minh, và mỗi cặp được kiểm bằng mắt trước khi ghi vào đây — cùng lý do
// và cùng khuôn với `capEnumDaKiem` (P3-70).
//
// # Một cặp đúng tên vẫn có thể SAI
//
// `OrderSummary` của đặc tả được phục vụ bởi `checkout.orderSummaryJSON`,
// KHÔNG phải một struct nào trong module `order`: nó là thứ trả về khi
// hoàn tất phiên thanh toán. Ghép theo tên module sẽ trỏ sai chỗ.
//
// # Vì sao sổ này KHÔNG phủ hết 36 schema
//
// Mười ba schema thuộc Phase 2 (`ContentSummary`, `Outfit`, `Review`,
// `CreatorRef`…) chưa có struct Go nào phục vụ. Ghép chúng là ghép với
// hư không.
//
// Số còn lại là các schema nhỏ được NHÚNG vào schema khác
// (`BrandRef`, `Image`, `ProductTag`) — chúng xuất hiện bên trong một
// struct lớn hơn, và tầng này so ở mức schema cấp cao nhất. Gác chúng cần
// một phép so lồng nhau; chưa làm, và `soDTOChuaGac` canh để việc chưa làm
// ấy không tăng lên.
var capDTODaKiem = []capDTO{
	// ---------------------------------------------------------- Mua hàng
	{
		Schema: "Offer", GoiGo: "modules/marketplace/interfaces/http",
		KieuGo: "offerJSON", LyDo: "lời chào bán một SKU",
	},
	{
		Schema: "SellerOffer", GoiGo: "modules/marketplace/interfaces/http",
		KieuGo: "sellerOfferJSON", LyDo: "offer nhìn từ phía nhà bán",
	},
	{
		Schema: "CartItem", GoiGo: "modules/cart/interfaces/http",
		KieuGo: "itemJSON", LyDo: "một dòng trong giỏ",
		ChoPhepThieu: map[string]string{
			// Tầng này tìm ra `product_id` ngày 25/09/2026 — trường hợp
			// THỨ SÁU của dạng lỗi nó sinh ra để bắt.
			//
			// Hậu quả nhìn thấy được: giỏ hàng không có đường về trang sản
			// phẩm. Khách muốn xem lại món mình đã thêm phải tự đi tìm.
			//
			// Không sửa ngay vì nó KHÔNG rẻ như bốn trường kia. Tầng HTTP
			// dựng `itemJSON` từ `domain.Item`, và mọi trường hiển thị của
			// Item đều được LƯU xuống `cart_item` (`product_name TEXT NOT
			// NULL DEFAULT ''`). Thêm `product_id` cho đúng khuôn ấy là
			// thêm một cột, tức một migration — và một migration không
			// thuộc phạm vi của việc dựng một hàng rào.
			//
			// Xem P3-80 để biết ba cách sửa và đánh đổi từng cách.
			"product_id": "cần migration thêm cột cart_item.product_id — P3-80",
		},
	},
	{
		Schema: "AttributionSource", GoiGo: "modules/cart/interfaces/http",
		KieuGo: "attributionJSON", LyDo: "nguồn giới thiệu của dòng giỏ",
	},

	// ------------------------------------------------- Phiên thanh toán
	//
	// Cặp này là lý do tầng thứ tư tồn tại: `shipping_groups` (P3-50) và
	// `shipping_method` (P3-73) đều từng vắng mặt ở đây.
	{
		Schema: "Checkout", GoiGo: "modules/checkout/interfaces/http",
		KieuGo: "checkoutJSON", LyDo: "phiên thanh toán — đã thiếu hai lần",
	},
	{
		Schema: "CheckoutLine", GoiGo: "modules/checkout/interfaces/http",
		KieuGo: "lineJSON", LyDo: "một dòng trong phiên thanh toán",
	},
	{
		Schema: "OrderSummary", GoiGo: "modules/checkout/interfaces/http",
		KieuGo: "orderSummaryJSON",
		LyDo:   "đơn vừa tạo, trả về khi hoàn tất phiên",
	},

	// ------------------------------------------------------- Đơn hàng
	{
		Schema: "OrderDetailLine", GoiGo: "modules/order/interfaces/http",
		KieuGo: "customerLineJSON", LyDo: "dòng đơn khách nhìn thấy",
	},

	// --------------------------------------------------------- Trả hàng
	{
		Schema: "ReturnRequest", GoiGo: "modules/returns/interfaces/http",
		KieuGo: "returnJSON", LyDo: "yêu cầu trả hàng",
	},
	{
		Schema: "ReturnLine", GoiGo: "modules/returns/interfaces/http",
		KieuGo: "returnLineJSON", LyDo: "một dòng xin trả",
	},

	// ------------------------------------------------------------ Tiền
	{
		Schema: "SellerBalance", GoiGo: "modules/payment/interfaces/http",
		KieuGo: "balanceJSON", LyDo: "số dư nhà bán",
	},
	{
		Schema: "Settlement", GoiGo: "modules/payment/interfaces/http",
		KieuGo: "settlementJSON",
		LyDo:   "đợt đối soát — `lines` đã từng bị vứt ở tầng DTO (P3-69)",
	},
}

// soDTOChuaGac là số schema CÓ thuộc tính mà chưa nằm trong sổ trên.
//
// # Vì sao một con số thay vì bắt khai hết
//
// Cùng lý do với `soEnumChuaGac`: phần lớn schema chưa gác thuộc Phase 2
// và không có struct Go nào để ghép. Bắt khai lý do cho từng cái sẽ sinh
// một sổ mười ba dòng "Phase 2" mà không ai đọc.
//
// Đây là CHỐT MỘT CHIỀU: thêm schema mới vào đặc tả mà không ghép cặp thì
// CI đỏ, buộc người thêm phải quyết. Nó CHỈ ĐƯỢC GIẢM.
//
// Nó KHÔNG phải lời hứa rằng phần còn lại đã được kiểm.
const soDTOChuaGac = 20
