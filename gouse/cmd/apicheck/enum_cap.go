package main

// Sổ ghi CẶP enum: đặc tả ⇄ hằng số Go.
//
// # Vì sao phải ghi tay từng cặp
//
// Ngày 17/09/2026 một phép rà tự động thử GHÉP THEO TÊN và cho hai cảnh
// báo GIẢ trên ba kết quả:
//
//	`OrderStatus` ↔ Go `Status`    hàng chục module đều có kiểu tên
//	                               `Status`; chúng gộp thành một tập khổng
//	                               lồ và mọi enum đều "thiếu" vài chục giá
//	                               trị
//	`ReturnReasonCode` ↔ `LyDo`    HAI kiểu `LyDo` khác nhau: một của
//	                               `returns`, một của `recommendation`
//
// Cảnh báo giả là thứ làm người ta tắt hẳn phép kiểm. Nên cặp phải tường
// minh, và mỗi cặp phải được KIỂM bằng mắt trước khi ghi vào đây.
//
// # Một cặp đúng tên vẫn có thể SAI
//
// `CartItem.availability` trông như ghép được với `cart/domain.ItemAvailability`
// — cùng khái niệm, cùng bốn giá trị. Nhưng tầng HTTP có hàm `availabilityJSON`
// DỊCH ở biên (`QUANTITY_REDUCED` → `LOW_STOCK`), nên giá trị miền KHÔNG
// phải giá trị trên dây. Ghép cặp ấy sẽ báo bốn lệch không có thật.
//
// Quy tắc: chỉ ghép khi giá trị miền ĐI THẲNG ra JSON.

// capEnum là các cặp đã KIỂM.
type capEnum struct {
	// Duong là đường dẫn trong đặc tả, dạng `file#/khóa/khóa/...`.
	//
	// Dùng đường dẫn thay vì số dòng: một lần sửa mô tả ở trên làm mọi số
	// dòng trôi, và một sổ phải sửa sau mỗi lần chỉnh chữ là sổ không ai
	// giữ đúng.
	Duong string

	// GoiGo và KieuGo trỏ tới khối hằng số trong `internal/`.
	GoiGo  string
	KieuGo string

	// ChoPhepTapCon: đặc tả được phép có ÍT hơn Go.
	//
	// Có thật: `tier` của `/me` không bao giờ trả `ANONYMIZED`, vì một
	// khách đã ẩn danh thì không đăng nhập được. Đặc tả hẹp hơn ở đây là
	// mô tả ĐÚNG, không phải thiếu sót.
	//
	// Chiều ngược lại KHÔNG bao giờ được phép: đặc tả khai một giá trị mà
	// Go không sinh ra là một nhánh client viết rồi không bao giờ chạy.
	ChoPhepTapCon bool

	// LyDo giải thích cặp này nói về cái gì, cho người đọc lỗi.
	LyDo string
}

var capEnumDaKiem = []capEnum{
	{"components/schemas.yaml#/OrderStatus", "modules/order/domain", "Status",
		false, "trạng thái đơn hàng"},
	{"components/schemas.yaml#/FulfillmentStatus", "modules/fulfillment/domain", "FOStatus",
		false, "trạng thái đơn thực hiện"},
	{"components/schemas.yaml#/Checkout/properties/status", "modules/checkout/domain", "Status",
		false, "trạng thái phiên thanh toán"},
	{"components/schemas.yaml#/PaymentMethod", "modules/order/domain", "PaymentMethod",
		false, "phương thức thanh toán"},
	{"components/schemas.yaml#/OrderLineSummary/properties/status", "modules/order/domain", "LineStatus",
		false, "trạng thái dòng đơn"},
	{"components/schemas.yaml#/LedgerLine/properties/account_type", "modules/payment/domain", "AccountType",
		false, "tài khoản sổ cái — đã lệch một lần, xem P3-61"},
	{"components/schemas.yaml#/LedgerLine/properties/direction", "modules/payment/domain", "Direction",
		false, "hướng bút toán"},
	{"components/schemas.yaml#/Settlement/properties/status", "modules/payment/domain", "TrangThaiDoiSoat",
		false, "trạng thái đợt đối soát — đã lệch một lần, xem P3-60"},
	{"components/schemas.yaml#/ProductDetail/properties/product_type", "modules/catalog/domain", "ProductType",
		false, "loại sản phẩm"},
	{"components/schemas.yaml#/ReturnReasonCode", "modules/returns/domain", "LyDo",
		false, "lý do trả hàng"},
	{"components/schemas.yaml#/ReturnRequest/properties/status", "modules/returns/domain", "TrangThai",
		false, "trạng thái yêu cầu trả hàng"},
	{"components/common.yaml#/schemas/ColorFamily", "modules/product/domain", "NhomMau",
		false, "nhóm màu — đã lệch GRAY/GREY, xem P3-70"},
	{"components/common.yaml#/schemas/SizeChart/properties/system", "modules/catalog/domain", "SizeSystem",
		false, "hệ size"},
	{"paths/admin.yaml#/AdminSellerSummary/properties/status", "modules/seller/domain", "Status",
		false, "trạng thái gian hàng (tham số lọc)"},
	{"paths/account.yaml#/me/get/responses/content/schema/properties/customer/properties/tier", "modules/customer/domain", "Status",
		true, "hạng khách — `/me` không trả ANONYMIZED vì khách đã ẩn danh thì không đăng nhập được"},
}

// enumKhongGhep là những enum CỐ Ý không ghép cặp, kèm lý do.
//
// Sổ này tồn tại để "không ghép" là một quyết định có người ký, không phải
// một chỗ bị bỏ quên.
var enumKhongGhep = map[string]string{
	"components/schemas.yaml#/CartItem/properties/availability": "tầng HTTP DỊCH ở biên " +
		"(`availabilityJSON`: QUANTITY_REDUCED → LOW_STOCK), nên giá trị miền không phải " +
		"giá trị trên dây",
	"components/schemas.yaml#/ProductDetail/properties/size_recommendation/properties/reason": "" +
		"`BODY_MEASUREMENTS` khai trong đặc tả mà chưa dùng được — khách chưa lưu số đo ở " +
		"đâu cả; xem chú thích ở product/interfaces/http/dto.go",
}

// soEnumChuaGac là số enum trong đặc tả CHƯA nằm trong hai sổ trên.
//
// # Vì sao một con số thay vì bắt khai hết
//
// Đặc tả có hơn tám mươi enum; phần lớn thuộc module Phase 2 chưa tồn tại
// (creator, content, campaign) nên không có hằng số Go nào để ghép. Bắt
// khai lý do cho từng cái sẽ sinh ra tám mươi dòng "Phase 2" mà không ai
// đọc — và một sổ không ai đọc là chỗ lỗi ẩn vào.
//
// Con số này là một CÁI CHỐT một chiều: thêm enum mới mà không ghép cặp thì
// CI đỏ, buộc người thêm phải quyết định có ghép hay không. Nó CHỈ ĐƯỢC
// GIẢM.
//
// Nó KHÔNG phải lời hứa rằng phần còn lại đã được kiểm. Công cụ in ra con
// số ấy ở chế độ `-v` để không ai nhầm.
const soEnumChuaGac = 63
