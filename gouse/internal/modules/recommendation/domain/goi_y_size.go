// Package domain là mô hình nghiệp vụ của việc gợi ý.
//
// # Phạm vi đợt đầu: ĐÚNG MỘT phương thức
//
// `size_recommendation` là trường đã khai trong `ProductDetail`, ở một
// endpoint đang chạy, có người dùng thật đọc — nên nó có bên gọi.
//
// Bốn phương thức còn lại của interface gợi ý (sản phẩm tương tự, xu
// hướng, hoàn thiện bộ, nội dung liên quan) KHÔNG có bên gọi nào: đặc tả
// API không khai `similar_products`, không có endpoint xu hướng. Dựng
// chúng bây giờ là tạo thêm một ca của dạng lỗi "khai mà không ai gọi".
package domain

import (
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// KetQua là điều khách NÓI RA về một size.
type KetQua string

const (
	// KetQuaDaMua: đã mua và không trả vì lý do size.
	//
	// Tín hiệu YẾU. Khách giữ hàng có thể vì vừa, cũng có thể vì ngại trả
	// — nên nó chỉ được dùng khi không có tín hiệu mạnh hơn.
	KetQuaDaMua KetQua = "DA_MUA"

	// KetQuaChat: trả hàng vì chật. Lần sau nên LÊN một size.
	KetQuaChat KetQua = "CHAT"

	// KetQuaRong: trả hàng vì rộng. Lần sau nên XUỐNG một size.
	KetQuaRong KetQua = "RONG"
)

// QuanSat là một lần khách nói ra điều gì đó về một size.
type QuanSat struct {
	Size   string
	KetQua KetQua

	// ThuTu là thứ tự thời gian: số LỚN hơn là mới hơn.
	ThuTu int64
}

// LyDo là căn cứ của một gợi ý, theo đúng enum của đặc tả API.
type LyDo string

const (
	LyDoDaMua   LyDo = "PREVIOUS_PURCHASE"
	LyDoTraHang LyDo = "RETURN_HISTORY"
)

// GoiY là kết quả gợi ý size.
type GoiY struct {
	Size string
	LyDo LyDo
}

// SuyLuanSize chọn size nên gợi ý, hoặc trả false nếu không đủ căn cứ.
//
// # Thứ tự ưu tiên, và vì sao TRẢ HÀNG thắng
//
// Một lần trả hàng vì size là khách CHỦ ĐỘNG bỏ công nói ra rằng size đó
// sai, và sai theo hướng nào. Một lần mua rồi im lặng thì mơ hồ hơn nhiều:
// có thể vừa, có thể họ ngại trả.
//
// Nên quan sát TRẢ HÀNG mới nhất thắng mọi lần mua. Chỉ khi chưa từng có
// phàn nàn nào về size thì mới dùng size đã mua.
//
// # Vì sao cần `cacSize` theo THỨ TỰ
//
// "Lên một size" chỉ có nghĩa khi biết thang size của sản phẩm đang xem.
// Thang ấy khác nhau theo loại hàng: S/M/L, 38/39/40, hay Free. Bên gọi —
// module `product` — là nơi biết thang đó, nên nó truyền vào.
//
// # Trả FALSE thay vì đoán
//
// Không có quan sát nào, hoặc size cần gợi ý nằm ngoài thang đang bán, thì
// KHÔNG gợi ý. Đặc tả khai trường này nullable đúng vì thế: một gợi ý sai
// còn tệ hơn không gợi ý, vì nó làm khách chọn nhầm rồi đổ lỗi cho nền
// tảng — và làm tăng đúng tỷ lệ hoàn hàng mà nó sinh ra để giảm.
func SuyLuanSize(quanSat []QuanSat, cacSize []string) (GoiY, bool) {
	thang := chuanHoaThang(cacSize)
	if len(thang) == 0 {
		return GoiY{}, false
	}

	var traMoiNhat *QuanSat
	var muaMoiNhat *QuanSat
	for i := range quanSat {
		q := &quanSat[i]
		switch q.KetQua {
		case KetQuaChat, KetQuaRong:
			if traMoiNhat == nil || q.ThuTu > traMoiNhat.ThuTu {
				traMoiNhat = q
			}
		case KetQuaDaMua:
			if muaMoiNhat == nil || q.ThuTu > muaMoiNhat.ThuTu {
				muaMoiNhat = q
			}
		}
	}

	if traMoiNhat != nil {
		buoc := 1
		if traMoiNhat.KetQua == KetQuaRong {
			buoc = -1
		}
		if s, ok := dichSize(thang, traMoiNhat.Size, buoc); ok {
			return GoiY{Size: s, LyDo: LyDoTraHang}, true
		}
		// Đã phàn nàn nhưng không dịch được — ví dụ size đó đã là lớn nhất
		// thang. KHÔNG rơi xuống "size đã mua": khách vừa nói size ấy sai,
		// gợi ý lại chính nó là phớt lờ điều họ nói.
		return GoiY{}, false
	}

	if muaMoiNhat != nil {
		if _, ok := viTri(thang, muaMoiNhat.Size); ok {
			return GoiY{Size: chuanHoa(muaMoiNhat.Size), LyDo: LyDoDaMua}, true
		}
	}

	return GoiY{}, false
}

// dichSize trả size cách `buoc` bậc trong thang, nếu còn trong thang.
func dichSize(thang []string, size string, buoc int) (string, bool) {
	i, ok := viTri(thang, size)
	if !ok {
		return "", false
	}
	j := i + buoc
	if j < 0 || j >= len(thang) {
		return "", false
	}
	return thang[j], true
}

func viTri(thang []string, size string) (int, bool) {
	s := chuanHoa(size)
	for i, v := range thang {
		if v == s {
			return i, true
		}
	}
	return 0, false
}

// chuanHoaThang bỏ size rỗng và trùng, GIỮ NGUYÊN thứ tự bên gọi đưa vào.
//
// Thứ tự là thông tin của bên gọi, không phải thứ tự chữ cái: "S, M, L"
// sắp theo bảng chữ cái sẽ thành "L, M, S" và mọi phép "lên một size" đảo
// chiều.
func chuanHoaThang(cacSize []string) []string {
	out := make([]string, 0, len(cacSize))
	daCo := make(map[string]bool, len(cacSize))
	for _, s := range cacSize {
		v := chuanHoa(s)
		if v == "" || daCo[v] {
			continue
		}
		daCo[v] = true
		out = append(out, v)
	}
	return out
}

func chuanHoa(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// QuanSatMoi là một quan sát chuẩn bị ghi xuống.
//
// Tách khỏi `QuanSat` vì hai thứ phục vụ hai chiều: cái này để GHI và mang
// đủ nguồn gốc để chống trùng; `QuanSat` để ĐỌC và chỉ mang thứ quy tắc
// suy luận cần.
type QuanSatMoi struct {
	CustomerID ids.ID
	BrandID    ids.ID
	Size       string
	KetQua     KetQua

	// NguonLoai và NguonID cho biết quan sát này đến từ đâu — khóa chống
	// trùng khi event được phát lại.
	NguonLoai string
	NguonID   ids.ID

	QuanSatLuc time.Time
}
