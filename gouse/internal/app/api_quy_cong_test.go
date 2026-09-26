package app

import (
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Nguồn QUY CÔNG phải đi hết đường: giỏ → phiên thanh toán → đơn hàng.
//
// # Một chuỗi có hai đầu mà không có giữa
//
// Hạ tầng quy công đã tồn tại ở CẢ HAI ĐẦU từ tháng 8:
//
//	cart_item.source_content_id       migration 000009
//	cart_item.source_creator_id       migration 000009, có index
//	order_line.attributed_creator_id  migration 000008, có index
//	order.PlaceOrderLineInput.AttributedCreatorID
//
// Và không gì nối hai đầu ấy: `checkout.domain.Line` không mang trường quy
// công, nên thông tin creator biến mất ở CHÍNH GIỮA chuỗi. Mọi cột và index
// nói trên chưa bao giờ có dữ liệu khác rỗng.
//
// Một hạ tầng hoàn chỉnh ở hai đầu và trống ở giữa — dạng lỗi hay gặp nhất
// của dự án này, lần này trải dài qua ba module. Nối ở migration 000057.
//
// # Vì sao bài này đi qua HTTP
//
// Chuỗi đi qua bốn module (cart → checkout → order, cộng marketplace để có
// offer) và ba tầng lưu trữ. Gọi thẳng service của từng module sẽ bỏ qua
// đúng chỗ đã đứt: tầng DTO và tầng persistence.
//
// # Vì sao bài này là PHÉP THỬ của ADR-0021
//
// ADR-0021 trả lời "xây Affiliate phải sửa Core bao nhiêu?" bằng một dự
// đoán. Bài này biến dự đoán thành con số: chuỗi quy công là khoản sửa
// core DUY NHẤT mà affiliate cần, và nó là khoản MỘT LẦN — creator,
// livestream, campaign sau này dùng lại chính chuỗi ấy.
func TestNguonQuyCongDiTuGioToiDon(t *testing.T) {
	a := newAPITest(t)

	offerID := a.timOfferBanDuoc()
	if offerID == "" {
		t.Skip("không có offer nào bán được")
	}

	// Hai mã KHÔNG cần tồn tại: module creator và content thuộc Phase 2.
	// Kernel chỉ CHỞ hai cái mã, không tra chúng — và đó đúng là điều cần
	// khẳng định: nếu kernel đi tra creator, nó đã biết về creator commerce.
	creatorID := ids.MustNew(ids.PrefixCreator).String()
	contentID := ids.MustNew(ids.PrefixContent).String()

	res := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
		"offer_id": offerID, "quantity": 1,
		"source": map[string]any{
			"creator_id": creatorID,
			"content_id": contentID,
		},
	}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ kèm nguồn: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	// Chặng 1: giỏ đã ghi nguồn.
	if n := a.demDong("cart_item",
		"source_creator_id = $1 AND source_content_id = $2",
		creatorID, contentID); n != 1 {
		t.Fatalf("cart_item ghi nguồn quy công: mong 1 dòng, nhận %d", n)
	}

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": emailMoi("quycong"),
		"guest_phone": "0900555444",
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	// Chặng 2 — CHỖ TỪNG ĐỨT.
	if n := a.demDong("checkout_line",
		"source_creator_id = $1 AND source_content_id = $2",
		creatorID, contentID); n != 1 {
		t.Fatalf("checkout_line mang nguồn quy công: mong 1 dòng, nhận %d — "+
			"đây là chỗ chuỗi từng đứt, xem migration 000057", n)
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Quy Công", "phone": "0900555444",
			"street_address": "3 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	if res := a.call(http.MethodPatch,
		"/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem()); res.code != http.StatusOK {
		t.Fatalf("chọn cách giao: HTTP %d — %s", res.code, res.raw)
	}
	if res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem()); res.code != http.StatusOK &&
		res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}

	// Chặng 3: đơn hàng nhận được quy công.
	if n := a.demDong("order_line", "attributed_creator_id = $1",
		creatorID); n != 1 {
		t.Fatalf("order_line nhận quy công: mong 1 dòng, nhận %d — chuỗi "+
			"giỏ→phiên→đơn chưa nối hết", n)
	}
}

// Kernel KHÔNG được tra module creator.
//
// Bài trên dùng hai mã KHÔNG tồn tại và vẫn phải xanh. Nếu một ngày nào đó
// kernel đi kiểm "creator này có thật không", bài trên sẽ đỏ — và đó là
// tín hiệu đúng: lúc ấy kernel đã biết về creator commerce, tức ranh giới
// ADR-0021 bị phá.
//
// Bài này khẳng định điều đó THÀNH LỜI, để người đọc biết mã không tồn tại
// là CHỦ Ý chứ không phải dữ liệu thử cẩu thả.
func TestKernelKhongTraCreator(t *testing.T) {
	a := newAPITest(t)

	offerID := a.timOfferBanDuoc()
	if offerID == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
		"offer_id": offerID, "quantity": 1,
		"source": map[string]any{
			"creator_id": ids.MustNew(ids.PrefixCreator).String(),
		},
	}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ với creator KHÔNG tồn tại phải được: HTTP %d — %s\n"+
			"Kernel chỉ CHỞ mã quy công. Đi tra nó nghĩa là kernel biết về "+
			"creator commerce, và ADR-0021 cấm điều đó.", res.code, res.raw)
	}
}
