package app

import (
	"net/http"
	"testing"
)

// TestPhuongThucThanhToanKhongBiNuot khóa lời hứa cơ bản nhất của một
// endpoint đặt hàng: thứ khách CHỌN phải là thứ hệ thống GHI.
//
// # Trạng thái trước khi sửa (P3-9)
//
// `payment_method` được kiểm tra hợp lệ rồi BỎ QUA — nó không bao giờ rời
// khỏi tầng HTTP. Đơn hàng không có trường nào ghi lại lựa chọn đó, nên
// khách chọn COD và khách chọn BANK_TRANSFER sinh ra hai đơn GIỐNG HỆT
// nhau. Không ai biết đơn nào phải thu tiền lúc giao.
//
// Kiểu hỏng này im lặng tuyệt đối: client gửi đúng, nhận 201, và trường
// bị vứt đi không để lại dấu vết nào.
func TestPhuongThucThanhToanKhongBiNuot(t *testing.T) {
	a := newAPITest(t)

	for _, pt := range []string{"COD", "BANK_TRANSFER"} {
		t.Run(pt, func(t *testing.T) {
			maPhien := a.dungPhienSanHoanTat("pttt-"+pt+"@example.com", "0900222111")
			res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
				map[string]any{"payment_method": pt}, khoaIdem())
			if res.code != http.StatusOK && res.code != http.StatusCreated {
				t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
			}
			don, _ := res.body["order"].(map[string]any)
			maDon, _ := don["id"].(string)
			if maDon == "" {
				t.Fatalf("không lấy được mã đơn: %s", res.raw)
			}

			// Phản hồi của chính lời gọi vừa tạo đơn phải nói lại lựa chọn.
			if got, _ := don["payment_method"].(string); got != pt {
				t.Errorf("phản hồi complete: payment_method = %q, mong %q", got, pt)
			}

			// Và ĐỌC LẠI đơn phải thấy đúng lựa chọn đó — đây mới là chỗ
			// chứng minh nó được GHI XUỐNG chứ chỉ dội lại trong bộ nhớ.
			chiTiet := a.call(http.MethodGet, "/api/v1/orders/"+maDon, nil,
				map[string]string{"X-Guest-Phone": "0900222111"})
			if chiTiet.code != http.StatusOK {
				t.Fatalf("đọc đơn: HTTP %d — %s", chiTiet.code, chiTiet.raw)
			}
			d, _ := chiTiet.body["order"].(map[string]any)
			if d == nil {
				d = chiTiet.body
			}
			if got, _ := d["payment_method"].(string); got != pt {
				t.Errorf("đọc lại đơn: payment_method = %q, mong %q — "+
					"lựa chọn của khách không được ghi xuống", got, pt)
			}
		})
	}
}

// TestThuLaiTraVePhuongThucCuaDonDaTao khóa một chi tiết dễ làm sai theo
// hướng "tiện hơn": dội lại thân request thay vì đọc từ đơn.
//
// Hai cách viết cho kết quả GIỐNG HỆT nhau ở đường thường, nên chỉ đường
// THỬ LẠI mới phân biệt được. Gọi lại cùng khóa idempotency với một phương
// thức KHÁC không tạo đơn thứ hai — đơn cũ được trả về — nên phương thức
// trong phản hồi phải là của ĐƠN, không phải cái vừa gửi.
//
// Dội lại thân request sẽ báo cho khách "đã ghi nhận BANK_TRANSFER" trong
// khi database ghi COD. Client thử lại sau timeout mạng là chuyện thường,
// và nó không cần phải gửi lại đúng thân cũ để nhận về sự thật.
func TestThuLaiTraVePhuongThucCuaDonDaTao(t *testing.T) {
	a := newAPITest(t)

	maPhien := a.dungPhienSanHoanTat("thulai-pttt@example.com", "0900555444")
	khoa := khoaIdem()

	dau := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoa)
	if dau.code != http.StatusOK && dau.code != http.StatusCreated {
		t.Fatalf("lần đầu: HTTP %d — %s", dau.code, dau.raw)
	}

	// CÙNG khóa idempotency, phương thức KHÁC.
	lai := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "BANK_TRANSFER"}, khoa)
	if lai.code != http.StatusOK && lai.code != http.StatusCreated {
		t.Fatalf("lần hai: HTTP %d — %s", lai.code, lai.raw)
	}

	don1, _ := dau.body["order"].(map[string]any)
	don2, _ := lai.body["order"].(map[string]any)

	// Cùng một đơn, không phải đơn thứ hai.
	if don1["id"] != don2["id"] {
		t.Fatalf("tạo đơn thứ hai: %v rồi %v — idempotency hỏng",
			don1["id"], don2["id"])
	}

	// Mã đơn cũng phải có mặt. Đây là mã khách đọc qua điện thoại và tra
	// đơn vãng lai bằng nó — trả rỗng ở đúng đường THỬ LẠI nghĩa là khách
	// gặp sự cố mạng thì mất luôn mã đơn của mình.
	if so, _ := don2["order_number"].(string); so == "" {
		t.Errorf("thử lại trả order_number rỗng (lần đầu: %q)", don1["order_number"])
	}

	if got, _ := don2["payment_method"].(string); got != "COD" {
		t.Errorf("thử lại trả payment_method = %q, mong COD — phản hồi phải "+
			"nói thứ ĐÃ GHI, không phải thứ vừa gửi", got)
	}
}
