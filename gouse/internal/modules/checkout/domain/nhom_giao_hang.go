package domain

import (
	"sort"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// PhiTheoNguon là phí và thời gian vận chuyển ước tính cho MỘT nguồn hàng.
//
// Domain nhận nó vào chứ không tự tính: biểu phí và số ngày vận chuyển
// thuộc về hãng vận chuyển, và module `fulfillment` mới là nơi biết.
type PhiTheoNguon struct {
	SellerID ids.ID

	// Phi là phí GỐC của nguồn này, trước khi xét miễn phí ship.
	Phi money.Money

	// SoNgay là thời gian VẬN CHUYỂN, KHÔNG gồm thời gian nhà bán chuẩn
	// bị hàng — xem `fulfillment/domain.MucPhi`.
	SoNgay int
}

// NhomGiaoHang là MỘT KIỆN: hàng của một nhà bán, đi riêng và tới riêng.
//
// # Vì sao khách phải thấy từng nhóm, không thấy một con số gộp
//
// docs/04-modules/checkout.md mục 7: "hiển thị thời gian giao RIÊNG cho
// từng nhóm hàng, không gộp thành một con số. Khách cần biết món nào đến
// trước." Một đơn trộn hàng ba nhà bán đi thành ba kiện, tốn ba lần phí và
// tới vào ba ngày khác nhau; gộp lại thành một dòng là giấu cả ba sự thật.
type NhomGiaoHang struct {
	SellerID   ids.ID
	SellerName string

	// PhiVanChuyen là phần phí của kiện này SAU khi đã xét miễn phí ship.
	// Tổng các nhóm luôn ĐÚNG BẰNG `Checkout.ShippingFee()`.
	PhiVanChuyen money.Money

	// NgayGiaoDuKien = ngày chuẩn bị xong + số ngày vận chuyển.
	NgayGiaoDuKien time.Time
}

// DatNhomGiaoHang dựng lại bảng kê theo nhà bán từ phí HIỆN TẠI của phiên.
//
// # Vì sao là trạng thái DẪN XUẤT, không phải trạng thái được lưu
//
// Bảng kê này không mang quyết định nào của riêng nó: phí đã nằm ở
// `shippingFee`, dòng hàng đã nằm ở `lines`, biểu phí đọc từ cấu hình vận
// hành trong bộ nhớ. Lưu thêm một bản chỉ tạo ra thứ thứ hai phải giữ cho
// khớp với thứ nhất.
//
// Và nó KHÔNG chia lại phí ước tính mà chia phí THỰC THU, nên tổng các
// nhóm luôn đúng bằng số khách trả kể cả khi biểu phí đổi giữa chừng.
func (c *Checkout) DatNhomGiaoHang(
	theoNguon []PhiTheoNguon, gioChuanBiMacDinh int, now time.Time,
) {
	c.nhomGiaoHang = dungNhomGiaoHang(
		c.lines, c.shippingFee, theoNguon, gioChuanBiMacDinh, now)
}

// dungNhomGiaoHang gom dòng hàng theo nhà bán rồi tính phí và ngày cho
// từng nhóm.
//
// # Thứ tự nhóm theo lần xuất hiện đầu của nhà bán trong giỏ
//
// Không sắp theo tên hay theo mã: khách nhìn thấy các nhóm theo đúng thứ
// tự họ đã bỏ hàng vào giỏ, và thứ tự đó ổn định giữa các lần tải trang.
//
// # Thời gian chuẩn bị lấy LỚN NHẤT trong nhóm
//
// Kiện chỉ đi khi món chậm nhất đã sẵn sàng. Lấy trung bình hay lấy món
// đầu tiên đều cho một lời hứa sớm hơn sự thật.
func dungNhomGiaoHang(
	lines []*Line,
	phi money.Money,
	theoNguon []PhiTheoNguon,
	gioChuanBiMacDinh int,
	now time.Time,
) []NhomGiaoHang {
	if len(lines) == 0 {
		return nil
	}

	type gom struct {
		sellerID ids.ID
		ten      string
		gioMax   int
	}

	var thuTu []ids.ID
	nhom := map[ids.ID]*gom{}
	for _, l := range lines {
		if !l.HasStock() {
			continue
		}
		g, co := nhom[l.SellerID()]
		if !co {
			g = &gom{sellerID: l.SellerID(), ten: l.SellerName()}
			nhom[l.SellerID()] = g
			thuTu = append(thuTu, l.SellerID())
		}
		if l.SellerName() != "" {
			g.ten = l.SellerName()
		}
		if h := l.HandlingTimeHours(); h > g.gioMax {
			g.gioMax = h
		}
	}
	if len(thuTu) == 0 {
		return nil
	}

	uoc := map[ids.ID]PhiTheoNguon{}
	for _, p := range theoNguon {
		uoc[p.SellerID] = p
	}

	// Chia phí THỰC THU, không dùng thẳng phí ước tính.
	//
	// Miễn phí ship (do ngưỡng hoặc do mã) đưa tổng về 0, và khi đó từng
	// nhóm cũng phải hiện 0. Chia từ con số thực thu khiến tổng các nhóm
	// LUÔN bằng số khách trả, kể cả khi biểu phí đổi giữa chừng.
	trongSo := make([]int64, len(thuTu))
	for i, id := range thuTu {
		trongSo[i] = uoc[id].Phi.Amount()
	}
	phanBo := chiaTheoTrongSo(phi.Amount(), trongSo)

	out := make([]NhomGiaoHang, 0, len(thuTu))
	for i, id := range thuTu {
		g := nhom[id]

		gio := g.gioMax
		if gio <= 0 {
			gio = gioChuanBiMacDinh
		}

		// Ngày chuẩn bị xong, quy về ĐẦU NGÀY theo giờ nghiệp vụ: lời hứa
		// giao hàng là một NGÀY, không phải một mốc giờ.
		xongChuanBi := types.DauNgay(now.Add(time.Duration(gio) * time.Hour))

		tien, err := money.New(phanBo[i], phi.Currency())
		if err != nil {
			tien = money.Zero(phi.Currency())
		}

		out = append(out, NhomGiaoHang{
			SellerID:       id,
			SellerName:     g.ten,
			PhiVanChuyen:   tien,
			NgayGiaoDuKien: xongChuanBi.AddDate(0, 0, uoc[id].SoNgay),
		})
	}
	return out
}

// chiaTheoTrongSo chia `tong` thành `len(trongSo)` phần theo tỷ lệ.
//
// # Vì sao dùng PHẦN DƯ LỚN NHẤT
//
// Chia số nguyên rồi làm tròn từng phần cho tổng các phần KHÁC tổng ban
// đầu — và ở đây sự chênh ấy nghĩa là bảng kê phí không cộng ra đúng số
// khách trả. Phần dư lớn nhất chia hết phần lẻ cho các nhóm xứng đáng
// nhất, nên tổng luôn khớp tuyệt đối.
//
// Trọng số bằng 0 hết (không ước tính được nguồn nào) thì chia ĐỀU: thà
// mỗi nhóm gánh một phần bằng nhau còn hơn dồn tất cả vào nhóm đầu.
func chiaTheoTrongSo(tong int64, trongSo []int64) []int64 {
	n := len(trongSo)
	out := make([]int64, n)
	if n == 0 || tong == 0 {
		return out
	}

	var sum int64
	for _, w := range trongSo {
		if w > 0 {
			sum += w
		}
	}
	if sum == 0 {
		for i := range out {
			out[i] = tong / int64(n)
		}
		for i := int64(0); i < tong%int64(n); i++ {
			out[i]++
		}
		return out
	}

	type du struct {
		i  int
		le int64
	}
	con := tong
	les := make([]du, 0, n)
	for i, w := range trongSo {
		if w <= 0 {
			continue
		}
		phan := tong * w / sum
		out[i] = phan
		con -= phan
		les = append(les, du{i: i, le: (tong * w) % sum})
	}

	// Phần lẻ lớn hơn được cộng trước; hòa thì nhóm đứng trước được ưu
	// tiên, để kết quả KHÔNG đổi giữa hai lần chạy cùng đầu vào.
	sort.SliceStable(les, func(a, b int) bool { return les[a].le > les[b].le })
	for i := 0; con > 0 && i < len(les); i++ {
		out[les[i].i]++
		con--
	}
	return out
}
