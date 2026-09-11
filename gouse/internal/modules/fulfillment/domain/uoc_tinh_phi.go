package domain

import (
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// ErrPhuongThucKhongHopLe: phương thức vận chuyển không nằm trong biểu phí.
var ErrPhuongThucKhongHopLe = errors.New(
	"fulfillment: phương thức vận chuyển không hợp lệ")

// PhuongThucGiao là cách hàng được chuyển tới khách.
type PhuongThucGiao string

const (
	GiaoTieuChuan PhuongThucGiao = "STANDARD"
	GiaoNhanh     PhuongThucGiao = "EXPRESS"
)

// MucPhi là phí và thời gian dự kiến của MỘT kiện hàng.
type MucPhi struct {
	// PhiMotNguon là phí cho MỘT nguồn hàng, không phải cho cả đơn.
	PhiMotNguon int64

	// SoNgayDuKien là thời gian vận chuyển, KHÔNG gồm thời gian nhà bán
	// chuẩn bị hàng. Hai con số đó thuộc hai bên khác nhau: hãng vận
	// chuyển chịu trách nhiệm phần này, nhà bán chịu phần kia
	// (`handling_time_hours` của offer).
	SoNgayDuKien int
}

// bieuPhi là biểu phí theo phương thức.
//
// # Vì sao nó nằm ở fulfillment chứ không ở checkout
//
// docs/04-modules/checkout.md mục 7 quy định phí đến từ
// `fulfillment.EstimateShipping()`. Trước 06/09 nó là một map hằng số
// TRONG checkout, và chú thích ở đó tự ghi rằng đấy là chỗ đứng tạm.
//
// Chỗ đúng là ở đây vì module này mới là nơi biết về hãng vận chuyển, lô
// hàng và điểm xuất hàng. Checkout chỉ cần biết con số phải thu.
//
// # Vì sao vẫn là bảng phẳng, và điều đó CHƯA đủ
//
// Phí thật phụ thuộc KHOẢNG CÁCH và KHỐI LƯỢNG. Cả hai đều chưa lấy được:
//
//	khoảng cách   `stock_location` không có địa chỉ — không có tỉnh, không
//	              có tọa độ. Không có điểm xuất hàng thì không có khoảng cách.
//	khối lượng    dòng checkout mang sku_id và số lượng, KHÔNG mang gram.
//
// Cả hai đều cần thêm dữ liệu chứ không chỉ thêm phép tính, nên chúng nằm
// ngoài P3-8. Xem ghi chú P3-8 trong backlog.
// BieuPhiGiao là bảng phí và thời gian theo phương thức vận chuyển.
type BieuPhiGiao map[PhuongThucGiao]MucPhi

// BieuPhiMacDinh dùng khi chưa nối cấu hình vận hành.
//
// # Vì sao nó là THAM SỐ chứ không còn là hằng số của gói
//
// Hai con số tiền là GIÁ HIỆN TRÊN MÀN HÌNH THANH TOÁN; hai con số ngày là
// LỜI HỨA GIAO HÀNG. Đổi chúng là việc chạy khuyến mãi, đàm phán lại với
// hãng, hoặc phản ứng với đối thủ — không việc nào nên chờ một lần triển
// khai.
//
// Bảng này ở lại làm LƯỚI CUỐI: domain phải đúng với mọi đầu vào, kể cả
// khi nối dây sai hoặc có bên gọi thứ hai không đi qua cấu hình.
var BieuPhiMacDinh = BieuPhiGiao{
	GiaoTieuChuan: {PhiMotNguon: 30_000, SoNgayDuKien: 3},
	GiaoNhanh:     {PhiMotNguon: 60_000, SoNgayDuKien: 1},
}

// NguonHang là một điểm xuất hàng — thực tế là MỘT KIỆN.
type NguonHang struct {
	SellerID string
}

// PhiTheoNguon là phí ước tính của một nguồn.
type PhiTheoNguon struct {
	SellerID     string
	Phi          money.Money
	SoNgayDuKien int
}

// UocTinhPhi là kết quả ước tính cho cả đơn.
type UocTinhPhi struct {
	Tong      money.Money
	TheoNguon []PhiTheoNguon
}

// UocTinhPhiGiao tính phí vận chuyển cho một đơn nhiều nguồn hàng.
//
// # MỖI NGUỒN LÀ MỘT KIỆN, và đó là toàn bộ điểm của hàm này
//
// Hàng của ba nhà bán khác nhau nằm ở ba kho khác nhau, nên nó đi thành BA
// kiện và tốn ba lần phí. Biểu phí phẳng trước đây thu MỘT lần cho cả đơn
// — nghĩa là mỗi đơn trộn nhiều nhà bán, nền tảng bù phần chênh, và bù
// càng nhiều khi đơn càng nhiều nguồn.
//
// Đó không phải sai sót nhỏ ở một cái chợ: đơn nhiều nhà bán là thứ cái
// chợ tồn tại để tạo ra.
//
// Đơn KHÔNG có nguồn nào trả phí 0 — không có gì để giao.
func UocTinhPhiGiao(
	phuongThuc PhuongThucGiao, nguon []NguonHang, donVi money.Currency,
	bieu BieuPhiGiao,
) (UocTinhPhi, error) {
	if bieu == nil {
		bieu = BieuPhiMacDinh
	}
	muc, ok := bieu[phuongThuc]
	if !ok {
		return UocTinhPhi{}, ErrPhuongThucKhongHopLe
	}

	tong := money.Zero(donVi)
	theoNguon := make([]PhiTheoNguon, 0, len(nguon))

	for _, n := range nguon {
		phi, err := money.New(muc.PhiMotNguon, donVi)
		if err != nil {
			return UocTinhPhi{}, err
		}
		tong, err = tong.Add(phi)
		if err != nil {
			return UocTinhPhi{}, err
		}
		theoNguon = append(theoNguon, PhiTheoNguon{
			SellerID:     n.SellerID,
			Phi:          phi,
			SoNgayDuKien: muc.SoNgayDuKien,
		})
	}

	return UocTinhPhi{Tong: tong, TheoNguon: theoNguon}, nil
}
