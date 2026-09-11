// Package opsconfig cho phép sửa MỘT SỐ tham số vận hành lúc đang chạy,
// thay vì phải build lại và triển khai lại.
//
// # Vì sao không phải mọi hằng số đều vào đây
//
// Đây là quyết định quan trọng nhất của gói này, và nó là quyết định về
// AN TOÀN chứ không phải về tiện lợi.
//
// Có hai loại hằng số trong hệ thống, và chúng trông giống hệt nhau:
//
//	chính sách kinh doanh   "hạn giao là 48 giờ"  — người kinh doanh quyết
//	kiểm soát đúng đắn      "lý do tối thiểu 20 ký tự" — bảo vệ hệ thống
//
// Loại thứ hai KHÔNG được vào đây. Một kiểm soát tự nới lỏng được từ giao
// diện quản trị thì không còn là kiểm soát: người muốn lách nó chỉ cần đổi
// một con số, và thao tác đổi đó cũng do chính họ ký.
//
// Ví dụ cụ thể của thứ KHÔNG bao giờ nên đưa vào:
//
//	audit.minReasonLen     hạ xuống 1 là vô hiệu hóa toàn bộ nhật ký truy cập
//	token.minSecretLen     hạ xuống là mở cửa cho khóa yếu
//	eventbus.maxAttempts   hạ xuống là đẩy hàng đợi vào dead letter
//	identity.MaxFailedAttempts  nâng lên là mở cửa cho dò mật khẩu
//
// # Vì sao SỔ ĐĂNG KÝ ĐÓNG
//
// Chỉ khóa khai ở đây mới tồn tại. Gõ sai tên khóa thì lỗi ngay, không tạo
// ra một tham số ma mà không ai đọc. Và không có đường nào để thêm khóa
// mới từ giao diện — thêm khóa là việc của người viết mã, có review.
package opsconfig

import (
	"fmt"
	"sort"
	"time"
)

// Kieu là kiểu dữ liệu của một tham số.
type Kieu string

const (
	KieuThoiLuong Kieu = "duration" // time.Duration, nhập bằng giờ
	KieuTyLe      Kieu = "ratio"    // 0..1
	KieuSoNguyen  Kieu = "int"
)

// Khóa của các tham số. Chuỗi thuần, KHÔNG import module nào — quy tắc R3
// của archcheck cấm platform biết tới module nghiệp vụ.
const (
	KeySLAGiaoHang       = "fulfillment.shipping_sla_hours"
	KeyNguongHuyDon      = "fulfillment.max_cancellation_rate"
	KeyNguongGiaoDungHan = "fulfillment.min_on_time_rate"
	KeyMauToiThieu       = "fulfillment.min_sample_size"

	KeyNguongBatTinGiaoHang = "fulfillment.delivery_silence_hours"

	// Giá NỀN TẢNG TRẢ hãng vận chuyển, mỗi kiện.
	//
	// KHÁC phí khách trả: chênh lệch giữa hai con số CHÍNH LÀ lãi/lỗ mảng
	// vận chuyển. Gán chúng bằng nhau là nói lãi bằng 0 mà không ai kiểm.
	KeyGiaHangTieuChuan = "fulfillment.carrier_cost_standard"
	KeyGiaHangNhanh     = "fulfillment.carrier_cost_express"

	// KeyHanDoiTra là thời hạn đổi trả, tính từ lúc GIAO XONG.
	//
	// MỘT nguồn cho HAI nơi: `fulfillment.CompleteDelivered` dùng nó để
	// biết khi nào chuyển tiền cho nhà bán, và `returns.XinTra` dùng nó
	// để từ chối yêu cầu quá hạn. Hai hằng số riêng nghĩa là sớm muộn
	// chúng lệch nhau — và lệch ở đây là hoàn tiền cho khách sau khi nhà
	// bán đã rút tiền.
	KeyHanDoiTra = "returns.window_hours"

	KeyTranSoLuongSKU = "inventory.max_quantity_per_sku"

	KeyThueSuat          = "checkout.tax_rate_bp"
	KeyNguongMienPhiShip = "checkout.free_shipping_threshold"

	// Trọng số công thức buy box.
	//
	// Đây là những con số quyết định NHÀ BÁN NÀO CÓ DOANH THU: buy box là
	// offer hiển thị mặc định, và phần lớn khách mua chính nó.
	//
	// Chúng là TRỌNG SỐ TƯƠNG ĐỐI, không phải phần trăm — công thức chia
	// cho tổng. Nên 40/30/30 và 4/3/3 cho cùng kết quả, và đổi MỘT khóa
	// không làm hệ thống rơi vào trạng thái không hợp lệ. Đó là lý do
	// không có ràng buộc "cộng đúng 100": một ràng buộc như thế sẽ khiến
	// mọi bước trung gian đều bị từ chối, và không ai đi từ 40/30/30 tới
	// 50/25/25 được nữa.
	KeyTrongSoGia      = "marketplace.buybox_weight_price"
	KeyTrongSoChuanBi  = "marketplace.buybox_weight_handling"
	KeyTrongSoHieuSuat = "marketplace.buybox_weight_performance"

	// KeyDiemHieuSuatMacDinh là điểm dùng cho nhà bán CHƯA có lịch sử.
	KeyDiemHieuSuatMacDinh = "marketplace.default_performance_score"

	// KeyGioChuanBiMacDinh là thời gian chuẩn bị hàng khi offer không khai.
	KeyGioChuanBiMacDinh = "marketplace.default_handling_hours"

	// Hoa hồng: mức ĐỀ XUẤT khi duyệt, và SÀN.
	//
	// Tỷ lệ của từng nhà bán là DỮ LIỆU (khác nhau theo hợp đồng), không
	// phải cấu hình. Hai con số ở đây là CHÍNH SÁCH quanh nó.
	KeyHoaHongDeXuat = "marketplace.commission_rate_default_bp"
	KeyHoaHongSan    = "marketplace.commission_rate_floor_bp"

	// Phí khách TRẢ cho vận chuyển, và số ngày hứa với khách.
	//
	// KHÁC `fulfillment.carrier_cost_*`: đó là tiền nền tảng TRẢ hãng.
	// Hiệu của hai con số là lãi/lỗ mảng vận chuyển.
	KeyPhiGiaoTieuChuan  = "fulfillment.shipping_fee_standard"
	KeyPhiGiaoNhanh      = "fulfillment.shipping_fee_express"
	KeyNgayGiaoTieuChuan = "fulfillment.shipping_days_standard"
	KeyNgayGiaoNhanh     = "fulfillment.shipping_days_express"

	// KeyNhipTaoDoiSoat là nhịp GOM bút toán thành đợt cho nhà bán.
	KeyNhipTaoDoiSoat = "payment.settlement_batch_interval_hours"
)

// TranLuuTruSoLuong là trần CỨNG của cột `quantity_*` trong database.
//
// Cột là `INT` của PostgreSQL — 32 bit. Đây là SỰ THẬT về nơi lưu, không
// phải lựa chọn: đặt cao hơn thì câu lệnh ghi hỏng và người dùng nhận 500.
//
// Nó là trần của trần: tham số nghiệp vụ bên dưới không bao giờ được vượt
// con số này, và `Max` trong sổ đăng ký cưỡng chế điều đó.
const TranLuuTruSoLuong = 2147483647

// ThamSo mô tả một tham số vận hành.
type ThamSo struct {
	Khoa string
	Kieu Kieu

	// MacDinh là giá trị dùng khi chưa ai đặt, HOẶC khi không đọc được
	// giá trị đã đặt.
	//
	// Với thời lượng, đơn vị là GIỜ; với tỷ lệ là 0..1; với số nguyên là
	// chính nó.
	MacDinh float64

	// Min, Max là biên chấp nhận được.
	//
	// Bắt buộc, không phải tùy chọn: một tham số không có biên là một
	// tham số ai đó sẽ đặt bằng 0 và làm sập một thứ ở xa.
	Min, Max float64

	// MoTa và HeQua hiện trên giao diện quản trị.
	//
	// HeQua nói điều gì xảy ra khi đổi — người đổi con số thường không
	// phải người viết đoạn mã đọc nó.
	MoTa  string
	HeQua string
}

// soDang ký là danh sách ĐÓNG các tham số sửa được.
var soDangKy = map[string]ThamSo{
	KeySLAGiaoHang: {
		Khoa: KeySLAGiaoHang, Kieu: KieuThoiLuong,
		MacDinh: 48, Min: 1, Max: 720,
		MoTa: "Hạn nhà bán phải bàn giao cho đơn vị vận chuyển, tính từ " +
			"lúc đơn thực hiện được tạo.",
		HeQua: "Đổi con số này làm ĐỔI ĐIỂM hiệu suất của mọi nhà bán ở kỳ " +
			"đang xem — kể cả những đơn đã giao xong từ trước. Hạ xuống " +
			"khiến nhiều gian hàng đột ngột bị chấm là giao trễ.",
	},
	KeyNguongHuyDon: {
		Khoa: KeyNguongHuyDon, Kieu: KieuTyLe,
		MacDinh: 0.03, Min: 0, Max: 1,
		MoTa:  "Tỷ lệ hủy đơn TỐI ĐA còn được coi là đạt.",
		HeQua: "Ngưỡng chặt hơn làm nhiều gian hàng chuyển sang CẢNH BÁO.",
	},
	KeyHanDoiTra: {
		Khoa: KeyHanDoiTra, Kieu: KieuThoiLuong,
		MacDinh: 168, Min: 24, Max: 2160,
		MoTa: "Thời hạn khách được đổi trả, tính từ lúc đơn giao xong. Hết " +
			"hạn thì số dư nhà bán chuyển sang khả dụng và yêu cầu trả hàng " +
			"bị từ chối.",
		HeQua: "Kéo dài thì tiền nhà bán nằm chờ lâu hơn; rút ngắn thì khách " +
			"mất quyền trả sớm hơn. ĐỔI CON SỐ NÀY ÁP CHO CẢ ĐƠN ĐÃ GIAO — " +
			"hạn tính ra từ mốc giao, không lưu sẵn, nên một lần rút ngắn có " +
			"thể làm những đơn đang trong hạn hết hạn ngay lập tức.",
	},
	KeyGiaHangTieuChuan: {
		Khoa: KeyGiaHangTieuChuan, Kieu: KieuSoNguyen,
		MacDinh: 0, Min: 0, Max: 100_000_000,
		MoTa: "Số tiền nền tảng TRẢ hãng vận chuyển cho MỘT kiện giao tiêu " +
			"chuẩn. 0 = CHƯA KHAI, và khi chưa khai thì không ghi bút toán " +
			"chi phí nào.",
		HeQua: "Đây là con số quyết định lãi/lỗ mảng vận chuyển. Đặt thấp " +
			"hơn thực tế làm doanh thu nền tảng trông đẹp hơn sự thật; để 0 " +
			"thì sổ cái tiếp tục thiếu vế chi phí và `cmd/doisoatso` sẽ báo.",
	},
	KeyGiaHangNhanh: {
		Khoa: KeyGiaHangNhanh, Kieu: KieuSoNguyen,
		MacDinh: 0, Min: 0, Max: 100_000_000,
		MoTa: "Số tiền nền tảng TRẢ hãng vận chuyển cho MỘT kiện giao nhanh. " +
			"0 = CHƯA KHAI.",
		HeQua: "Xem fulfillment.carrier_cost_standard.",
	},
	KeyNguongBatTinGiaoHang: {
		Khoa: KeyNguongBatTinGiaoHang, Kieu: KieuThoiLuong,
		MacDinh: 168, Min: 24, Max: 720,
		MoTa: "Gói hàng đã bàn giao mà KHÔNG có cập nhật nào lâu hơn khoảng " +
			"này thì bị coi là mất tin và đưa vào danh sách đi hỏi đơn vị " +
			"vận chuyển. Đây là ngưỡng NGHI NGỜ, không phải hạn giao hàng.",
		HeQua: "Hạ xuống thì bắt được webhook mất sớm hơn, nhưng những gói " +
			"đang giao bình thường ở tuyến xa cũng bị gọi tên — và một " +
			"cảnh báo luôn kêu thì không ai đọc. Nâng lên thì tiền của nhà " +
			"bán nằm im lâu hơn trước khi có người phát hiện.",
	},
	KeyNguongGiaoDungHan: {
		Khoa: KeyNguongGiaoDungHan, Kieu: KieuTyLe,
		MacDinh: 0.95, Min: 0, Max: 1,
		MoTa:  "Tỷ lệ giao đúng hạn TỐI THIỂU để được coi là đạt.",
		HeQua: "Ngưỡng cao hơn làm nhiều gian hàng chuyển sang CẢNH BÁO.",
	},
	KeyTranSoLuongSKU: {
		Khoa: KeyTranSoLuongSKU, Kieu: KieuSoNguyen,
		MacDinh: 10_000_000, Min: 1, Max: TranLuuTruSoLuong,
		MoTa: "Số lượng tối đa cho MỘT SKU tại MỘT kho, dùng khi kiểm kê " +
			"hoặc điều chỉnh thủ công.",
		HeQua: "Đây là trần NGHIỆP VỤ, đặt thấp hơn trần lưu trữ " +
			"(2.147.483.647) có chủ ý: nó bắt lỗi gõ thừa số 0 ngay lúc " +
			"nhập, thay vì để một con số vô lý nằm trong kho và làm sai " +
			"mọi báo cáo tồn. Nâng lên chỉ khi có mặt hàng thật sự đếm " +
			"bằng đơn vị nhỏ (chỉ, cúc, hạt cườm).",
	},
	KeyThueSuat: {
		Khoa: KeyThueSuat, Kieu: KieuSoNguyen,
		MacDinh: 800, Min: 0, Max: 10000,
		MoTa: "Thuế suất tính theo PHẦN VẠN (800 = 8%). Một tầng, áp cho " +
			"mọi mặt hàng và cho cả phí vận chuyển.",
		HeQua: "Đây là con số đi vào TỔNG TIỀN khách trả và vào sổ cái. " +
			"Đổi nó KHÔNG sửa lại đơn cũ — đơn đã đặt giữ thuế suất tại " +
			"thời điểm đặt, vì tiền trên đơn là hợp đồng đã đóng băng. " +
			"Đặt về 0 nghĩa là không thu thuế, và hóa đơn xuất ra sẽ ghi 0.",
	},
	KeyNguongMienPhiShip: {
		Khoa: KeyNguongMienPhiShip, Kieu: KieuSoNguyen,
		MacDinh: 499_000, Min: 0, Max: 1_000_000_000,
		MoTa: "Tiền hàng TỐI THIỂU (sau giảm giá, chưa gồm phí ship và " +
			"thuế) để được miễn phí vận chuyển. Áp trên TỔNG ĐƠN.",
		HeQua: "Đặt về 0 là MIỄN PHÍ SHIP CHO MỌI ĐƠN — kể cả đơn 10.000đ " +
			"gửi từ ba nhà bán, tức nền tảng chịu ba lần phí. Đó là một " +
			"lựa chọn hợp lệ trong đợt khuyến mãi, nhưng gõ nhầm một số 0 " +
			"thì không có gì chặn lại.",
	},
	KeyTrongSoGia: {
		Khoa: KeyTrongSoGia, Kieu: KieuSoNguyen,
		MacDinh: 40, Min: 0, Max: 100,
		MoTa: "Trọng số của GIÁ trong công thức buy box. Tương đối, không " +
			"phải phần trăm — công thức chia cho tổng ba trọng số.",
		HeQua: "Nâng giá lên quá nửa tổng là mở ra cuộc đua giảm giá: một " +
			"offer rẻ hơn 10% thắng được offer tốt hơn ở CẢ HAI tiêu chí " +
			"còn lại, nên nhà bán hạ giá tới mức không bền rồi cắt chất " +
			"lượng phục vụ để bù. Hạ xuống 0 là bỏ hẳn giá khỏi công thức: " +
			"khách không còn được lợi khi nhà bán cạnh tranh giá.",
	},
	KeyTrongSoChuanBi: {
		Khoa: KeyTrongSoChuanBi, Kieu: KieuSoNguyen,
		MacDinh: 30, Min: 0, Max: 100,
		MoTa: "Trọng số của THỜI GIAN CHUẨN BỊ HÀNG trong công thức buy box.",
		HeQua: "Nâng lên thì nhà bán ở xa hoặc làm hàng thủ công khó thắng " +
			"buy box dù giá tốt. Hạ xuống 0 thì khách không còn được ưu " +
			"tiên nhận hàng sớm.",
	},
	KeyTrongSoHieuSuat: {
		Khoa: KeyTrongSoHieuSuat, Kieu: KieuSoNguyen,
		MacDinh: 30, Min: 0, Max: 100,
		MoTa: "Trọng số của ĐIỂM HIỆU SUẤT nhà bán trong công thức buy box.",
		HeQua: "Hạ xuống 0 là nói nhà bán giao trễ và nhà bán giao đúng hạn " +
			"ngang nhau — và khi đó không còn động lực nào để giữ chất lượng.",
	},
	KeyDiemHieuSuatMacDinh: {
		Khoa: KeyDiemHieuSuatMacDinh, Kieu: KieuSoNguyen,
		MacDinh: 50, Min: 0, Max: 100,
		MoTa: "Điểm hiệu suất dùng cho nhà bán CHƯA có lịch sử giao hàng.",
		HeQua: "Đây là lựa chọn TĂNG TRƯỞNG, không phải con số kỹ thuật. " +
			"Đặt thấp thì nhà bán mới gần như không bán được đơn đầu tiên " +
			"— mà không có đơn đầu tiên thì không bao giờ có lịch sử để " +
			"thoát ra. Đặt cao thì người mới ngang người đã chứng minh " +
			"được mình, và khách chịu rủi ro thay.",
	},
	KeyGioChuanBiMacDinh: {
		Khoa: KeyGioChuanBiMacDinh, Kieu: KieuSoNguyen,
		MacDinh: 24, Min: 1, Max: 720,
		MoTa: "Thời gian chuẩn bị hàng (giờ) áp cho offer KHÔNG tự khai.",
		HeQua: "Con số này vào CẢ điểm buy box lẫn ngày giao dự kiến báo " +
			"cho khách. Đặt thấp là hứa thay cho nhà bán một điều họ chưa " +
			"cam kết.",
	},
	KeyHoaHongDeXuat: {
		Khoa: KeyHoaHongDeXuat, Kieu: KieuSoNguyen,
		MacDinh: 0, Min: 0, Max: 10000,
		MoTa: "Tỷ lệ hoa hồng ĐỀ XUẤT khi duyệt nhà bán mới, theo điểm cơ " +
			"bản (1200 = 12%). 0 = không đề xuất gì, người duyệt tự điền.",
		HeQua: "Chỉ là con số ĐIỀN SẴN trên màn hình duyệt — nó không tự áp " +
			"cho ai. Nhưng phần lớn người duyệt sẽ giữ nguyên giá trị điền " +
			"sẵn, nên thực tế nó là tỷ lệ của đa số nhà bán mới.",
	},
	KeyHoaHongSan: {
		Khoa: KeyHoaHongSan, Kieu: KieuSoNguyen,
		MacDinh: 0, Min: 0, Max: 10000,
		MoTa: "SÀN hoa hồng: duyệt nhà bán dưới mức này bị TỪ CHỐI. " +
			"0 = không có sàn (hành vi trước khi có tham số này).",
		HeQua: "Sàn 0 nghĩa là duyệt nhầm một nhà bán ở 0% thì nền tảng " +
			"không thu được đồng nào trên mọi đơn của họ, và không gì báo. " +
			"Nhà bán OWN BRAND luôn được miễn sàn: nền tảng không thu hoa " +
			"hồng của chính mình, và một ràng buộc ở database cưỡng chế " +
			"điều đó.",
	},
	KeyPhiGiaoTieuChuan: {
		Khoa: KeyPhiGiaoTieuChuan, Kieu: KieuSoNguyen,
		MacDinh: 30_000, Min: 0, Max: 100_000_000,
		MoTa: "Phí vận chuyển KHÁCH TRẢ cho một nguồn hàng, giao tiêu chuẩn.",
		HeQua: "Đây là con số hiện trên màn hình thanh toán. Đặt THẤP HƠN " +
			"`fulfillment.carrier_cost_standard` là lỗ trên mỗi kiện, và " +
			"không có gì trong hệ thống chặn điều đó — hãy xem hai con số " +
			"cạnh nhau.",
	},
	KeyPhiGiaoNhanh: {
		Khoa: KeyPhiGiaoNhanh, Kieu: KieuSoNguyen,
		MacDinh: 60_000, Min: 0, Max: 100_000_000,
		MoTa:  "Phí vận chuyển KHÁCH TRẢ cho một nguồn hàng, giao nhanh.",
		HeQua: "Xem fulfillment.shipping_fee_standard.",
	},
	KeyNgayGiaoTieuChuan: {
		Khoa: KeyNgayGiaoTieuChuan, Kieu: KieuSoNguyen,
		MacDinh: 3, Min: 1, Max: 60,
		MoTa: "Số ngày vận chuyển dự kiến, giao tiêu chuẩn. KHÔNG gồm thời " +
			"gian nhà bán chuẩn bị hàng.",
		HeQua: "Đây là LỜI HỨA với khách: nó hiện lúc thanh toán và thành " +
			"ngày giao dự kiến trên trang theo dõi đơn. Rút ngắn mà hãng " +
			"vận chuyển không nhanh hơn thì mọi đơn đều trông như trễ.",
	},
	KeyNgayGiaoNhanh: {
		Khoa: KeyNgayGiaoNhanh, Kieu: KieuSoNguyen,
		MacDinh: 1, Min: 1, Max: 60,
		MoTa:  "Số ngày vận chuyển dự kiến, giao nhanh.",
		HeQua: "Xem fulfillment.shipping_days_standard.",
	},
	KeyNhipTaoDoiSoat: {
		Khoa: KeyNhipTaoDoiSoat, Kieu: KieuThoiLuong,
		MacDinh: 1, Min: 1, Max: 168,
		MoTa: "Nhịp GOM bút toán thành đợt đối soát cho nhà bán. Việc CHI " +
			"TRẢ vẫn do người duyệt, nên đây không phải một kiểm soát tiền.",
		HeQua: "Nhịp thưa hơn thì nhà bán thấy khoản của mình muộn hơn — " +
			"đó là câu họ hỏi nhiều nhất. Nhịp này cũng được công bố cho " +
			"luật cảnh báo, nên đổi nó sẽ tự nới ngưỡng báo job treo.",
	},
	KeyMauToiThieu: {
		Khoa: KeyMauToiThieu, Kieu: KieuSoNguyen,
		MacDinh: 10, Min: 1, Max: 10000,
		MoTa: "Số đơn tối thiểu trong kỳ thì mới chấm hiệu suất.",
		HeQua: "Hạ xuống quá thấp là chấm gian hàng mới mở bằng vài đơn — " +
			"một lần hủy thành tỷ lệ 33% và bị đánh giá NGHIÊM TRỌNG.",
	},
}

// Tham tra một tham số theo khóa.
func Tham(khoa string) (ThamSo, bool) {
	t, ok := soDangKy[khoa]
	return t, ok
}

// MoiThamSo trả toàn bộ sổ đăng ký, sắp theo khóa.
func MoiThamSo() []ThamSo {
	ra := make([]ThamSo, 0, len(soDangKy))
	for _, t := range soDangKy {
		ra = append(ra, t)
	}
	sort.Slice(ra, func(i, j int) bool { return ra[i].Khoa < ra[j].Khoa })
	return ra
}

// KiemGiaTri kiểm một giá trị có hợp lệ cho tham số này không.
//
// Kiểm ở đây chứ không ở tầng HTTP: mọi đường ghi đều phải đi qua, kể cả
// đường nạp dữ liệu hay một công cụ dòng lệnh sau này.
func (t ThamSo) KiemGiaTri(v float64) error {
	if v < t.Min || v > t.Max {
		return fmt.Errorf(
			"%w: %s phải trong khoảng [%g, %g], nhận %g",
			ErrNgoaiBien, t.Khoa, t.Min, t.Max, v)
	}
	if t.Kieu == KieuSoNguyen && v != float64(int64(v)) {
		return fmt.Errorf("%w: %s phải là số nguyên, nhận %g",
			ErrSaiKieu, t.Khoa, v)
	}
	return nil
}

// ThoiLuong đổi giá trị (giờ) thành time.Duration.
func (t ThamSo) ThoiLuong(v float64) time.Duration {
	return time.Duration(v * float64(time.Hour))
}
