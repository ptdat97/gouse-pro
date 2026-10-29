package domain

import (
	"sort"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// SourceFee là phí và thời gian vận chuyển ước tính cho MỘT nguồn hàng.
//
// Domain nhận nó vào chứ không tự tính: biểu phí và số ngày vận chuyển
// thuộc về hãng vận chuyển, và module `fulfillment` mới là nơi biết.
type SourceFee struct {
	SellerID ids.ID

	// Phi là phí GỐC của nguồn này, trước khi xét miễn phí ship.
	Phi money.Money

	// Days là thời gian VẬN CHUYỂN, KHÔNG gồm thời gian nhà bán chuẩn
	// bị hàng — xem `fulfillment/domain.FeeTier`.
	Days int
}

// ShippingGroup là MỘT KIỆN: hàng của một nhà bán, đi riêng và tới riêng.
//
// # Vì sao khách phải thấy từng nhóm, không thấy một con số gộp
//
// docs/04-modules/checkout.md mục 7: "hiển thị thời gian giao RIÊNG cho
// từng nhóm hàng, không gộp thành một con số. Khách cần biết món nào đến
// trước." Một đơn trộn hàng ba nhà bán đi thành ba kiện, tốn ba lần phí và
// tới vào ba ngày khác nhau; gộp lại thành một dòng là giấu cả ba sự thật.
type ShippingGroup struct {
	SellerID   ids.ID
	SellerName string

	// ShippingFee là phần phí của kiện này SAU khi đã xét miễn phí ship.
	// Tổng các nhóm luôn ĐÚNG BẰNG `Checkout.ShippingFee()`.
	ShippingFee money.Money

	// EstimatedDelivery = ngày chuẩn bị xong + số ngày vận chuyển.
	EstimatedDelivery time.Time
}

// SetShippingGroups dựng lại bảng kê theo nhà bán từ phí HIỆN TẠI của phiên.
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
func (c *Checkout) SetShippingGroups(
	perSource []SourceFee, defaultHandlingHours int, now time.Time,
) {
	c.shippingGroups = buildShippingGroups(
		c.lines, c.shippingFee, perSource, defaultHandlingHours, now)
}

// buildShippingGroups gom dòng hàng theo nhà bán rồi tính phí và ngày cho
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
func buildShippingGroups(
	lines []*Line,
	fee money.Money,
	perSource []SourceFee,
	defaultHandlingHours int,
	now time.Time,
) []ShippingGroup {
	if len(lines) == 0 {
		return nil
	}

	type group struct {
		sellerID ids.ID
		name     string
		maxHours int
	}

	var sellerOrder []ids.ID
	groups := map[ids.ID]*group{}
	for _, l := range lines {
		if !l.HasStock() {
			continue
		}
		g, co := groups[l.SellerID()]
		if !co {
			g = &group{sellerID: l.SellerID(), name: l.SellerName()}
			groups[l.SellerID()] = g
			sellerOrder = append(sellerOrder, l.SellerID())
		}
		if l.SellerName() != "" {
			g.name = l.SellerName()
		}
		if h := l.HandlingTimeHours(); h > g.maxHours {
			g.maxHours = h
		}
	}
	if len(sellerOrder) == 0 {
		return nil
	}

	estimate := map[ids.ID]SourceFee{}
	for _, p := range perSource {
		estimate[p.SellerID] = p
	}

	// Chia phí THỰC THU, không dùng thẳng phí ước tính.
	//
	// Miễn phí ship (do ngưỡng hoặc do mã) đưa tổng về 0, và khi đó từng
	// nhóm cũng phải hiện 0. Chia từ con số thực thu khiến tổng các nhóm
	// LUÔN bằng số khách trả, kể cả khi biểu phí đổi giữa chừng.
	weights := make([]int64, len(sellerOrder))
	for i, id := range sellerOrder {
		weights[i] = estimate[id].Phi.Amount()
	}
	allocations := splitByWeight(fee.Amount(), weights)

	out := make([]ShippingGroup, 0, len(sellerOrder))
	for i, id := range sellerOrder {
		g := groups[id]

		hours := g.maxHours
		if hours <= 0 {
			hours = defaultHandlingHours
		}

		// Ngày chuẩn bị xong, quy về ĐẦU NGÀY theo giờ nghiệp vụ: lời hứa
		// giao hàng là một NGÀY, không phải một mốc giờ.
		readyAt := types.DauNgay(now.Add(time.Duration(hours) * time.Hour))

		fee, err := money.New(allocations[i], fee.Currency())
		if err != nil {
			fee = money.Zero(fee.Currency())
		}

		out = append(out, ShippingGroup{
			SellerID:          id,
			SellerName:        g.name,
			ShippingFee:       fee,
			EstimatedDelivery: readyAt.AddDate(0, 0, estimate[id].Days),
		})
	}
	return out
}

// splitByWeight chia `total` thành `len(weights)` phần theo tỷ lệ.
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
func splitByWeight(total int64, weights []int64) []int64 {
	n := len(weights)
	out := make([]int64, n)
	if n == 0 || total == 0 {
		return out
	}

	var sum int64
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum == 0 {
		for i := range out {
			out[i] = total / int64(n)
		}
		for i := int64(0); i < total%int64(n); i++ {
			out[i]++
		}
		return out
	}

	type remainder struct {
		i    int
		frac int64
	}
	rest := total
	fracs := make([]remainder, 0, n)
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		share := total * w / sum
		out[i] = share
		rest -= share
		fracs = append(fracs, remainder{i: i, frac: (total * w) % sum})
	}

	// Phần lẻ lớn hơn được cộng trước; hòa thì nhóm đứng trước được ưu
	// tiên, để kết quả KHÔNG đổi giữa hai lần chạy cùng đầu vào.
	sort.SliceStable(fracs, func(a, b int) bool { return fracs[a].frac > fracs[b].frac })
	for i := 0; rest > 0 && i < len(fracs); i++ {
		out[fracs[i].i]++
		rest--
	}
	return out
}
