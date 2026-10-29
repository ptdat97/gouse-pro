package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	fulfillmentapp "github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
)

// Hoa hồng creator đi HẾT chuỗi: giỏ → phiên → đơn → tiền về → hoa hồng.
//
// # Bài này là PHÉP THỬ CHẤP NHẬN của ADR-0021
//
// ADR-0021 nói: dựng một module extension thật phải cần 0 dòng sửa ở module
// kernel. `TestTheAttributionSourceTravelsFromCartToOrder` đã kiểm chặng 1 — kernel CHỞ được
// mã creator qua ba module. Bài này kiểm chặng 2: module `affiliate` biến mã
// ấy thành TIỀN, và nó làm được mà không module nào biết nó tồn tại.
//
// Ba điều được khẳng định, theo thứ tự quan trọng:
//
//  1. Quy công xảy ra lúc TIỀN VỀ, không lúc đặt đơn
//  2. Tiền gốc là tiền THỰC TRẢ, và hoa hồng tính từ tỷ lệ ĐÓNG BĂNG
//  3. Hoa hồng bắt đầu ở CHỜ ĐẾN HẠN, chỉ rút được khi hết hạn đổi trả
//
// # Vì sao đi qua HTTP và event thật
//
// Chuỗi đi qua năm module (cart → checkout → order → payment →
// fulfillment) rồi mới tới affiliate, và affiliate nhận dữ liệu qua HAI
// đường khác nhau: event `order.paid` cho tín hiệu, và cổng công khai của
// order cho dòng hàng. Gọi thẳng service của affiliate sẽ bỏ qua đúng chỗ
// có thể đứt — `OrderLineView.AttributedCreatorID` là một trường vừa mới
// được bày ra, và trước đó dữ liệu đã nằm trong bảng suốt mà không ai lấy
// được.
func TestCreatorCommissionGoesEndToEnd(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	creatorID := ids.MustNew(ids.PrefixCreator).String()
	orderID, tong, foID, sellerID, _, contentID := a.creatorPrepaidOrder(t, creatorID)

	// ---- Chặng 1: đặt đơn xong mà CHƯA quy công ----
	//
	// `checkout.completed` đã phát và đã tới mọi bên nhận. Nếu affiliate
	// nghe event ấy thay vì `order.paid`, số dưới đây sẽ là 1 — và đó là
	// một khoản hoa hồng cho doanh thu chưa tồn tại.
	a.emitEvent(t)
	if n := a.countAttributions(t, orderID); n != 0 {
		t.Fatalf("đơn CHƯA trả tiền đã có %d bản quy công, cần 0 — "+
			"hoa hồng phải sinh ra lúc TIỀN VỀ, không lúc đặt đơn", n)
	}

	// ---- Chặng 2: tiền về → quy công ----
	whRes := a.callPaymentWebhook(t, "cong-tt", map[string]any{
		"event_id":   "evt_hoa_hong_creator_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_hoa_hong_1",
			"amount":            tong,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": orderID},
		},
	}, paymentGatewaySecret)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.emitEvent(t)

	if n := a.countAttributions(t, orderID); n != 1 {
		t.Fatalf("sau khi tiền về: %d bản quy công, cần 1 — chuỗi "+
			"order.paid → affiliate chưa nối, hoặc "+
			"OrderLineView.AttributedCreatorID không mang mã creator", n)
	}

	// Bản quy công phải ĐÓNG BĂNG đủ thứ để đối soát lại được.
	var (
		gotCreator, gotContent, gotSeller string
		bearer, model                     string
		tyLe                              int
		baseAmount                        int64
	)
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT creator_id, content_id, seller_id, cost_bearer,
		       attribution_model, commission_rate, commission_base
		  FROM attribution WHERE order_id = $1`, orderID,
	).Scan(&gotCreator, &gotContent, &gotSeller, &bearer, &model,
		&tyLe, &baseAmount); err != nil {
		t.Fatalf("đọc bản quy công: %v", err)
	}

	if gotCreator != creatorID {
		t.Errorf("quy công cho creator %q, cần %q", gotCreator, creatorID)
	}

	// content_id đi hết chuỗi: giỏ → phiên → đơn → bản quy công.
	//
	// Tới migration 000060 cột này LUÔN RỖNG vì `order_line` không có chỗ
	// chở nó, nên câu "video nào bán được hàng" không trả lời được (P3-84).
	// Khẳng định ở đây chứ không chỉ ở `order_line`: chặng cuối là chặng
	// affiliate thật sự đọc, và một bản cài chở đúng tới `order_line` mà
	// đánh rơi ở adapter vẫn xanh nếu chỉ kiểm chặng trước.
	if gotContent != contentID {
		t.Errorf("bản quy công ghi nội dung %q, cần %q — chuỗi nội dung "+
			"chưa đi hết tới attribution", gotContent, contentID)
	}
	// seller_id CẦN để mở khóa theo từng nhà bán — xem chặng 4.
	if gotSeller != sellerID {
		t.Errorf("bản quy công ghi nhà bán %q, cần %q — thiếu nó thì một đơn "+
			"nhiều nhà bán sẽ mở khóa hoa hồng của cả đơn khi nhà bán đầu "+
			"tiên giao xong", gotSeller, sellerID)
	}
	if tyLe != 500 {
		t.Errorf("tỷ lệ hoa hồng đóng băng = %d điểm cơ bản, cần 500", tyLe)
	}
	if bearer != "PLATFORM" {
		t.Errorf("bên chịu chi phí = %q, cần PLATFORM — đoán sang nhà bán là "+
			"lấy tiền của người ngoài công ty", bearer)
	}
	if model != "LAST_CLICK" {
		t.Errorf("mô hình quy công = %q, cần LAST_CLICK — phải GHI RA để đổi "+
			"mô hình sau này còn giải thích được số cũ", model)
	}

	// ---- Chặng 3: số tiền, và trạng thái ĐẦU ----
	//
	// Khẳng định phép tính, không chỉ sự tồn tại: một bản ghi có mặt với số
	// tiền sai là tệ hơn không có bản ghi nào.
	var (
		amount int64
		tt     string
	)
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT c.amount, c.status
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1`, orderID).Scan(&amount, &tt); err != nil {
		t.Fatalf("đọc khoản hoa hồng: %v", err)
	}

	if mong := baseAmount * 500 / 10000; amount != mong {
		t.Errorf("hoa hồng = %d, cần %d (5%% của tiền gốc %d) — số tiền phải "+
			"TÍNH từ tỷ lệ và tiền gốc đã đóng băng", amount, mong, baseAmount)
	}
	if tt != "PENDING" {
		t.Fatalf("trạng thái đầu = %q, cần PENDING — hoa hồng chỉ rút được "+
			"sau khi hết hạn đổi trả (affiliate.md quy tắc 4)", tt)
	}

	// Tiền gốc là tiền THỰC TRẢ của dòng, không phải tổng đơn: tổng đơn có
	// cả phí vận chuyển, mà creator không tạo ra khoản phí ấy.
	if baseAmount >= tong {
		t.Errorf("tiền gốc tính hoa hồng = %d, tổng đơn = %d — tiền gốc phải "+
			"là tiền HÀNG của dòng, không gồm phí vận chuyển", baseAmount, tong)
	}

	// ---- Chặng 4: hết hạn đổi trả → RÚT ĐƯỢC ----
	tokNB := a.createSellerToken(t, sellerID)
	for _, steps := range []string{"ship", "deliver"} {
		than := map[string]any{}
		if steps == "ship" {
			than = map[string]any{
				"shipping_provider": "GHN", "tracking_number": "GHN-HH-1",
			}
		}
		res := a.call(http.MethodPost,
			"/api/v1/seller/fulfillment-orders/"+foID+"/"+steps, than,
			merge(idemKey(), map[string]string{
				"Authorization": "Bearer " + tokNB,
			}))
		if res.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", steps, res.code, res.raw)
		}
	}
	a.emitEvent(t)

	// Giao xong nhưng CHƯA hết hạn trả: hoa hồng vẫn phải CHỜ.
	if tt := a.commissionStatus(t, orderID); tt != "PENDING" {
		t.Fatalf("giao hàng xong mà hoa hồng đã %q — hạn đổi trả chưa qua, "+
			"tiền chưa được mở", tt)
	}

	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - $1::interval
		  WHERE id = $2`,
		(fulfillmentapp.ReturnWindow + time.Hour).String(), foID); err != nil {
		t.Fatalf("đẩy mốc giao: %v", err)
	}
	if n, err := a.mods.fulfillment.CompleteDelivered(ctx, 10); err != nil || n == 0 {
		t.Fatalf("hoàn tất đơn quá hạn: n=%d err=%v", n, err)
	}
	a.emitEvent(t)

	if tt := a.commissionStatus(t, orderID); tt != "AVAILABLE" {
		t.Fatalf("hết hạn đổi trả mà hoa hồng vẫn %q — creator không bao giờ "+
			"được chi, cùng dạng lỗi số dư nhà bán đứng vĩnh viễn", tt)
	}

	// PHÁT LẠI event KHÔNG được đổi gì: `fulfillment_order.completed` giao
	// ít nhất một lần.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL, attempts = 0
		  WHERE event_type = 'fulfillment_order.completed'`); err != nil {
		t.Fatalf("đưa event về hàng chờ: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx,
		`DELETE FROM event_processed
		  WHERE handler LIKE 'affiliate.%'`); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
	a.emitEvent(t)

	if n := a.countAttributions(t, orderID); n != 1 {
		t.Errorf("phát lại event sinh thêm bản quy công: %d, cần 1 — "+
			"UNIQUE (order_line_id) là thứ chặn thật", n)
	}
}

// Hủy đơn phải ĐẢO hoa hồng, kèm lý do.
//
// affiliate.md quy tắc 5. Không có đường này thì hủy đơn để lại một khoản
// hoa hồng CHỜ ĐẾN HẠN cho doanh thu không còn tồn tại — và tới kỳ chi trả,
// nền tảng trả tiền cho một đơn đã hủy. Im lặng, và là tiền.
func TestCancellingAnOrderReversesTheCreatorCommission(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	creatorID := ids.MustNew(ids.PrefixCreator).String()
	orderID, tong, _, _, phone, _ := a.creatorPrepaidOrder(t, creatorID)

	whRes := a.callPaymentWebhook(t, "cong-tt", map[string]any{
		"event_id":   "evt_hoa_hong_dao_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_hoa_hong_dao_1",
			"amount":            tong,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": orderID},
		},
	}, paymentGatewaySecret)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.emitEvent(t)

	if tt := a.commissionStatus(t, orderID); tt != "PENDING" {
		t.Fatalf("trước khi hủy: hoa hồng = %q, cần PENDING", tt)
	}

	// Hủy đơn qua ĐƯỜNG CỦA KHÁCH, không gọi service: đường ấy là đường duy
	// nhất phát `order.cancelled`.
	//
	// `reason` thuộc danh sách ĐÓNG năm giá trị của đặc tả, và khách vãng lai
	// chứng minh đơn là của mình bằng số điện thoại ở header.
	res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/cancel",
		map[string]any{"reason": "CHANGED_MIND"},
		merge(idemKey(), map[string]string{"X-Guest-Phone": phone}))
	if res.code != http.StatusOK {
		t.Fatalf("hủy đơn: HTTP %d — %s", res.code, res.raw)
	}
	a.emitEvent(t)

	if tt := a.commissionStatus(t, orderID); tt != "REVERSED" {
		t.Fatalf("hủy đơn mà hoa hồng vẫn %q — nền tảng sẽ trả tiền cho một "+
			"đơn đã hủy", tt)
	}

	// Lý do BẮT BUỘC có, và phải là lý do THẬT của đơn.
	var reason string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT c.reversed_reason
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1`, orderID).Scan(&reason); err != nil {
		t.Fatalf("đọc lý do đảo: %v", err)
	}
	if reason == "" {
		t.Error("đảo mà không ghi lý do — một khoản tiền biến mất không giải " +
			"thích được là điểm mù trong kiểm toán")
	}

	// Đã đảo thì KHÔNG mở khóa lại được, dù tín hiệu hết hạn có tới.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL, attempts = 0
		  WHERE event_type = 'order.cancelled'`); err != nil {
		t.Fatalf("đưa event về hàng chờ: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx,
		`DELETE FROM event_processed WHERE handler LIKE 'affiliate.%'`,
	); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
	a.emitEvent(t)

	if tt := a.commissionStatus(t, orderID); tt != "REVERSED" {
		t.Errorf("phát lại order.cancelled đổi trạng thái thành %q — đảo phải "+
			"IDEMPOTENT", tt)
	}
}

// ------------------------------------------------------------ tiện ích

// creatorPrepaidOrder dựng một đơn TRẢ TRƯỚC từ nội dung của creator.
//
// TRẢ TRƯỚC chứ không COD: chỉ đơn trả trước đi qua `order.paid` ngay, còn
// COD phải chờ tới lúc giao. Bài test cần mốc tiền về ở giữa chuỗi.
func (a *apiTest) creatorPrepaidOrder(
	t *testing.T, creatorID string,
) (orderID string, tong int64, foID, sellerID, phone, contentID string) {
	t.Helper()

	maOffer := a.findSellableOffer()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	contentID = ids.MustNew(ids.PrefixContent).String()
	res := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
		"offer_id": maOffer, "quantity": 1,
		"source": map[string]any{
			"creator_id": creatorID,
			"content_id": contentID,
		},
	}, idemKey())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ kèm nguồn creator: HTTP %d — %s", res.code, res.raw)
	}
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	// Số điện thoại RIÊNG cho mỗi đơn: nó là thứ duy nhất chứng minh đơn
	// thuộc về khách vãng lai này, và cả gói test dùng chung một database.
	phone = "0900666" + ids.MustNew(ids.PrefixCreator).String()[24:27]
	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id":     cartID,
		"guest_email": newEmail("hoahong"),
		"guest_phone": phone,
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Hoa Hồng", "phone": phone,
			"street_address": "7 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "CARD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ = ord["id"].(string)
	if orderID == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}
	tongMap, _ := ord["total"].(map[string]any)
	amount, _ := tongMap["amount"].(float64)
	tong = int64(amount)

	// Đơn thực hiện chỉ tồn tại sau khi `checkout.completed` được phát.
	a.emitEvent(t)
	foID, sellerID = a.fulfillmentOfOrder(t, orderID)
	return orderID, tong, foID, sellerID, phone, contentID
}

func (a *apiTest) countAttributions(t *testing.T, orderID string) int {
	t.Helper()
	return a.countRows("attribution", "order_id = $1", orderID)
}

// commissionStatus trả trạng thái khoản hoa hồng DUY NHẤT của một đơn.
//
// Đỏ khi có nhiều hơn một: bài test dựng đơn MỘT dòng, nên hai khoản nghĩa
// là quy công đã chạy hai lần — thứ phải thấy ngay chứ không phải đọc dòng
// đầu rồi bỏ qua.
func (a *apiTest) commissionStatus(t *testing.T, orderID string) string {
	t.Helper()

	rows, err := a.db.Pool().Query(context.Background(), `
		SELECT c.status
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1`, orderID)
	if err != nil {
		t.Fatalf("đọc trạng thái hoa hồng: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("đọc trạng thái hoa hồng: %v", err)
		}
		got = append(got, s)
	}
	if len(got) != 1 {
		t.Fatalf("đơn %s có %d khoản hoa hồng (%v), cần đúng 1", orderID, len(got), got)
	}
	return got[0]
}

// Đơn HAI nhà bán: mở khóa theo TỪNG nhà bán, không theo cả đơn.
//
// # Vì sao bài này tồn tại
//
// Tín hiệu hết hạn đổi trả là `fulfillment_order.completed`, và nó mang
// `(order_id, seller_id)` — một đơn nhiều nhà bán phát nó NHIỀU LẦN, mỗi
// nhà bán một lần, vào những thời điểm khác nhau.
//
// Nếu affiliate bỏ qua `seller_id` thì lần phát ĐẦU TIÊN mở khóa hoa hồng
// của cả đơn. Hàng của nhà bán thứ hai còn trong hạn đổi trả, nhưng tiền
// hoa hồng của nó đã vào diện chi được. Khách trả hàng sau đó thì nền tảng
// đã chi tiền cho một dòng bị hoàn — đúng việc đòi lại tiền đã trả mà quy
// tắc 4 của affiliate.md được viết ra để tránh.
//
// Bài một-nhà-bán ở trên KHÔNG bắt được lỗi đó: với một nhà bán, mở theo đơn
// và mở theo nhà bán cho cùng kết quả.
func TestCommissionUnlocksPerSeller(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	orderID, dsFO := a.paidTwoSellerOrder(t, "2nb")

	// Hoàn tất đơn thực hiện của MỘT nhà bán thôi.
	one := dsFO[0]
	tokNB := a.createSellerToken(t, one.sellerID)
	for _, steps := range []string{"ship", "deliver"} {
		than := map[string]any{}
		if steps == "ship" {
			than = map[string]any{
				"shipping_provider": "GHN", "tracking_number": "GHN-2NB-1",
			}
		}
		if r := a.call(http.MethodPost,
			"/api/v1/seller/fulfillment-orders/"+one.id+"/"+steps, than,
			merge(idemKey(), map[string]string{
				"Authorization": "Bearer " + tokNB,
			})); r.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", steps, r.code, r.raw)
		}
	}
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - $1::interval
		  WHERE id = $2`,
		(fulfillmentapp.ReturnWindow + time.Hour).String(), one.id); err != nil {
		t.Fatalf("đẩy mốc giao: %v", err)
	}
	if n, err := a.mods.fulfillment.CompleteDelivered(ctx, 10); err != nil || n == 0 {
		t.Fatalf("hoàn tất đơn quá hạn: n=%d err=%v", n, err)
	}
	a.emitEvent(t)

	// ĐÚNG MỘT khoản mở, đúng của nhà bán đã hoàn tất.
	openPerSeller := func(sellerID string) string {
		var tt string
		if err := a.db.Pool().QueryRow(ctx, `
			SELECT c.status
			  FROM commission_record c
			  JOIN attribution a ON a.id = c.attribution_id
			 WHERE a.order_id = $1 AND a.seller_id = $2`,
			orderID, sellerID).Scan(&tt); err != nil {
			t.Fatalf("đọc hoa hồng của nhà bán %s: %v", sellerID, err)
		}
		return tt
	}

	if tt := openPerSeller(one.sellerID); tt != "AVAILABLE" {
		t.Errorf("nhà bán đã hết hạn đổi trả mà hoa hồng vẫn %q", tt)
	}
	if tt := openPerSeller(dsFO[1].sellerID); tt != "PENDING" {
		t.Fatalf("hoa hồng của nhà bán CHƯA giao xong đã %q — mở khóa bỏ qua "+
			"seller_id, nên hàng còn trong hạn đổi trả đã vào diện chi được", tt)
	}
}

// openCheckoutFromCart mở phiên thanh toán từ một giỏ và điền đủ địa chỉ, cách giao.
//
// Tách khỏi `creatorPrepaidOrder` vì bài hai nhà bán tự dựng giỏ: nó cần
// hai offer, và ghép việc dựng giỏ vào cùng một tiện ích sẽ cho một hàm có
// hai chế độ.
func (a *apiTest) openCheckoutFromCart(t *testing.T, cartID, email, phone string) string {
	t.Helper()

	res := a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": email, "guest_phone": phone,
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Hoa Hồng", "phone": phone,
			"street_address": "9 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())

	return checkoutID
}

// fulfillmentOrder là một đơn thực hiện trong bài test, kèm nhà bán của nó.
type fulfillmentOrder struct{ id, sellerID string }

// paidTwoSellerOrder dựng một đơn HAI nhà bán, cùng một creator, ĐÃ trả
// tiền và ĐÃ quy công.
//
// CÙNG một creator cho cả hai dòng có chủ ý: bài dùng nó kiểm ranh giới NHÀ
// BÁN, nên hai creator khác nhau sẽ làm lẫn hai nguyên nhân với nhau.
//
// `label` phân biệt mã idempotency của webhook giữa các bài: cả gói test dùng
// chung một database, nên hai bài gửi cùng `event_id` thì bài sau nhận lại
// phản hồi của bài trước và không có event nào được phát.
func (a *apiTest) paidTwoSellerOrder(
	t *testing.T, label string,
) (orderID string, dsFO []fulfillmentOrder) {
	t.Helper()
	ctx := context.Background()

	offerA, offerB := a.twoOffersDifferentSellers(t)
	if offerA == "" || offerB == "" {
		t.Skip("không có đủ hai nhà bán bán được hàng")
	}

	creatorID := ids.MustNew(ids.PrefixCreator).String()
	for _, id := range []string{offerA, offerB} {
		r := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
			"offer_id": id, "quantity": 1,
			"source": map[string]any{"creator_id": creatorID},
		}, idemKey())
		if r.code != http.StatusOK {
			t.Fatalf("thêm offer %s: HTTP %d — %s", id, r.code, r.raw)
		}
	}

	cartBody := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	c, _ := cartBody.body["cart"].(map[string]any)
	if group, _ := c["groups"].([]any); len(group) < 2 {
		t.Skipf("giỏ chỉ có %d nhà bán, cần 2", len(group))
	}
	cartID, _ := c["id"].(string)

	phone := "09004" + ids.MustNew(ids.PrefixRequest).String()[25:30]
	checkoutID := a.openCheckoutFromCart(t, cartID, newEmail("hh"+label), phone)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "CARD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ = ord["id"].(string)
	tongMap, _ := ord["total"].(map[string]any)
	amount, _ := tongMap["amount"].(float64)

	// Phát TRƯỚC webhook: đơn thực hiện chỉ tồn tại sau `checkout.completed`.
	a.emitEvent(t)

	whRes := a.callPaymentWebhook(t, "cong-tt", map[string]any{
		"event_id":   "evt_hoa_hong_" + label,
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_hoa_hong_" + label,
			"amount":            int64(amount),
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": orderID},
		},
	}, paymentGatewaySecret)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.emitEvent(t)

	if n := a.countAttributions(t, orderID); n != 2 {
		t.Fatalf("đơn hai nhà bán có %d bản quy công, cần 2 — quy công theo "+
			"TỪNG DÒNG, không theo cả đơn", n)
	}

	rows, err := a.db.Pool().Query(ctx,
		`SELECT id, seller_id FROM fulfillment_order WHERE order_id = $1
		  ORDER BY created_at`, orderID)
	if err != nil {
		t.Fatalf("đọc đơn thực hiện: %v", err)
	}
	for rows.Next() {
		var f fulfillmentOrder
		if err := rows.Scan(&f.id, &f.sellerID); err != nil {
			t.Fatalf("đọc đơn thực hiện: %v", err)
		}
		dsFO = append(dsFO, f)
	}
	rows.Close()
	if len(dsFO) != 2 {
		t.Fatalf("đơn tách thành %d đơn thực hiện, cần 2", len(dsFO))
	}

	return orderID, dsFO
}

// commissionStatusBySeller trả trạng thái khoản hoa hồng của một nhà bán
// trong một đơn.
func (a *apiTest) commissionStatusBySeller(
	t *testing.T, orderID, sellerID string,
) string {
	t.Helper()
	var tt string
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT c.status
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1 AND a.seller_id = $2`,
		orderID, sellerID).Scan(&tt); err != nil {
		t.Fatalf("đọc hoa hồng của nhà bán %s: %v", sellerID, err)
	}
	return tt
}

// Hủy đơn thực hiện của MỘT nhà bán phải ĐẢO hoa hồng của đúng dòng ấy.
//
// # P3-85 — một dòng rác trong bảng tiền, và nó im lặng
//
// Tới 01/10/2026 đường hủy TỪNG PHẦN không đi qua chỗ nào affiliate nghe
// được:
//
//	đơn thực hiện của MỘT nhà bán bị hủy
//	  → fulfillment phát `fulfillment.cancelled`
//	  → order.RecalculateStatus → PARTIALLY_CANCELLED
//	  → KHÔNG phát `order.cancelled`
//
// Hệ quả KHÔNG phải trả tiền sai — đơn thực hiện bị hủy không bao giờ phát
// `fulfillment_order.completed`, nên khoản ấy cũng không mở khóa. Nó đứng ở
// CHỜ ĐẾN HẠN **mãi mãi**: không đảo, không chi, không ai thấy.
//
// Bài này đòi hai điều cùng lúc, và điều thứ hai mới là chỗ dễ sai: dòng của
// nhà bán CÒN LẠI không được chạm tới. `ReverseByOrder` đảo theo `order_id`, nên
// dùng nó cho việc này sẽ xanh ở nửa đầu và sai ở nửa sau.
func TestCancellingAFulfillmentReversesThatLinesCommission(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	orderID, dsFO := a.paidTwoSellerOrder(t, "huy_fo")
	cancelled, left := dsFO[0], dsFO[1]

	for _, f := range dsFO {
		if tt := a.commissionStatusBySeller(t, orderID, f.sellerID); tt != "PENDING" {
			t.Fatalf("trước khi hủy: hoa hồng của %s là %q, cần PENDING",
				f.sellerID, tt)
		}
	}

	// Hủy qua API CỦA MODULE: không có route HTTP nào cho việc này, và đây
	// là đường production thật — `CancelFulfillment` là thứ phát
	// `fulfillment.cancelled`.
	if err := a.mods.fulfillment.CancelFulfillment(
		ctx, cancelled.sellerID, cancelled.id, "hết hàng",
	); err != nil {
		t.Fatalf("hủy đơn thực hiện: %v", err)
	}
	a.emitEvent(t)

	if tt := a.commissionStatusBySeller(t, orderID, cancelled.sellerID); tt != "REVERSED" {
		t.Fatalf("nhà bán hủy mà hoa hồng vẫn %q — khoản này sẽ đứng ở "+
			"CHỜ ĐẾN HẠN mãi mãi, vì đơn thực hiện bị hủy không bao giờ phát "+
			"fulfillment_order.completed (P3-85)", tt)
	}

	// VÀ dòng của nhà bán còn lại KHÔNG được chạm.
	if tt := a.commissionStatusBySeller(t, orderID, left.sellerID); tt != "PENDING" {
		t.Fatalf("hoa hồng của nhà bán VẪN ĐANG GIAO đã thành %q — đảo theo "+
			"order_id thay vì theo dòng đơn", tt)
	}

	// Lý do phải nêu rõ ĐƠN THỰC HIỆN nào: "đơn bị hủy" không tra ra được
	// chuyện đã xảy ra lúc đối soát.
	var reason string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT c.reversed_reason
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1 AND a.seller_id = $2`,
		orderID, cancelled.sellerID).Scan(&reason); err != nil {
		t.Fatalf("đọc lý do đảo: %v", err)
	}
	if !strings.Contains(reason, "đơn thực hiện") {
		t.Errorf("lý do đảo = %q — phải nêu rõ đơn thực hiện nào bị hủy", reason)
	}

	// Phát LẠI event không được đổi gì: giao ít nhất một lần.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL, attempts = 0
		  WHERE event_type = 'fulfillment.cancelled'`); err != nil {
		t.Fatalf("đưa event về hàng chờ: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx,
		`DELETE FROM event_processed WHERE handler LIKE 'affiliate.%'`,
	); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
	a.emitEvent(t)

	if tt := a.commissionStatusBySeller(t, orderID, left.sellerID); tt != "PENDING" {
		t.Errorf("phát lại event đổi hoa hồng của nhà bán còn lại thành %q", tt)
	}
}
