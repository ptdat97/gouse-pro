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
// kernel. `TestNguonQuyCongDiTuGioToiDon` đã kiểm chặng 1 — kernel CHỞ được
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
func TestHoaHongCreatorDiHetChuoi(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	creatorID := ids.MustNew(ids.PrefixCreator).String()
	maDon, tong, foID, sellerID, _ := a.donTraTruocCuaCreator(t, creatorID)

	// ---- Chặng 1: đặt đơn xong mà CHƯA quy công ----
	//
	// `checkout.completed` đã phát và đã tới mọi bên nhận. Nếu affiliate
	// nghe event ấy thay vì `order.paid`, số dưới đây sẽ là 1 — và đó là
	// một khoản hoa hồng cho doanh thu chưa tồn tại.
	a.phatEvent(t)
	if n := a.demQuyCong(t, maDon); n != 0 {
		t.Fatalf("đơn CHƯA trả tiền đã có %d bản quy công, cần 0 — "+
			"hoa hồng phải sinh ra lúc TIỀN VỀ, không lúc đặt đơn", n)
	}

	// ---- Chặng 2: tiền về → quy công ----
	whRes := a.goiWebhookThanhToan(t, "cong-tt", map[string]any{
		"event_id":   "evt_hoa_hong_creator_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_hoa_hong_1",
			"amount":            tong,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": maDon},
		},
	}, biMatCongTT)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.phatEvent(t)

	if n := a.demQuyCong(t, maDon); n != 1 {
		t.Fatalf("sau khi tiền về: %d bản quy công, cần 1 — chuỗi "+
			"order.paid → affiliate chưa nối, hoặc "+
			"OrderLineView.AttributedCreatorID không mang mã creator", n)
	}

	// Bản quy công phải ĐÓNG BĂNG đủ thứ để đối soát lại được.
	var (
		gotCreator, gotSeller, benChiu, moHinh string
		tyLe                                   int
		tienGoc                                int64
	)
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT creator_id, seller_id, cost_bearer, attribution_model,
		       commission_rate, commission_base
		  FROM attribution WHERE order_id = $1`, maDon,
	).Scan(&gotCreator, &gotSeller, &benChiu, &moHinh, &tyLe, &tienGoc); err != nil {
		t.Fatalf("đọc bản quy công: %v", err)
	}

	if gotCreator != creatorID {
		t.Errorf("quy công cho creator %q, cần %q", gotCreator, creatorID)
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
	if benChiu != "PLATFORM" {
		t.Errorf("bên chịu chi phí = %q, cần PLATFORM — đoán sang nhà bán là "+
			"lấy tiền của người ngoài công ty", benChiu)
	}
	if moHinh != "LAST_CLICK" {
		t.Errorf("mô hình quy công = %q, cần LAST_CLICK — phải GHI RA để đổi "+
			"mô hình sau này còn giải thích được số cũ", moHinh)
	}

	// ---- Chặng 3: số tiền, và trạng thái ĐẦU ----
	//
	// Khẳng định phép tính, không chỉ sự tồn tại: một bản ghi có mặt với số
	// tiền sai là tệ hơn không có bản ghi nào.
	var (
		soTien int64
		tt     string
	)
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT c.amount, c.status
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1`, maDon).Scan(&soTien, &tt); err != nil {
		t.Fatalf("đọc khoản hoa hồng: %v", err)
	}

	if mong := tienGoc * 500 / 10000; soTien != mong {
		t.Errorf("hoa hồng = %d, cần %d (5%% của tiền gốc %d) — số tiền phải "+
			"TÍNH từ tỷ lệ và tiền gốc đã đóng băng", soTien, mong, tienGoc)
	}
	if tt != "PENDING" {
		t.Fatalf("trạng thái đầu = %q, cần PENDING — hoa hồng chỉ rút được "+
			"sau khi hết hạn đổi trả (affiliate.md quy tắc 4)", tt)
	}

	// Tiền gốc là tiền THỰC TRẢ của dòng, không phải tổng đơn: tổng đơn có
	// cả phí vận chuyển, mà creator không tạo ra khoản phí ấy.
	if tienGoc >= tong {
		t.Errorf("tiền gốc tính hoa hồng = %d, tổng đơn = %d — tiền gốc phải "+
			"là tiền HÀNG của dòng, không gồm phí vận chuyển", tienGoc, tong)
	}

	// ---- Chặng 4: hết hạn đổi trả → RÚT ĐƯỢC ----
	tokNB := a.taoTokenNhaBan(t, sellerID)
	for _, buoc := range []string{"ship", "deliver"} {
		than := map[string]any{}
		if buoc == "ship" {
			than = map[string]any{
				"shipping_provider": "GHN", "tracking_number": "GHN-HH-1",
			}
		}
		res := a.call(http.MethodPost,
			"/api/v1/seller/fulfillment-orders/"+foID+"/"+buoc, than,
			hopNhat(khoaIdem(), map[string]string{
				"Authorization": "Bearer " + tokNB,
			}))
		if res.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", buoc, res.code, res.raw)
		}
	}
	a.phatEvent(t)

	// Giao xong nhưng CHƯA hết hạn trả: hoa hồng vẫn phải CHỜ.
	if tt := a.trangThaiHoaHong(t, maDon); tt != "PENDING" {
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
	a.phatEvent(t)

	if tt := a.trangThaiHoaHong(t, maDon); tt != "AVAILABLE" {
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
	a.phatEvent(t)

	if n := a.demQuyCong(t, maDon); n != 1 {
		t.Errorf("phát lại event sinh thêm bản quy công: %d, cần 1 — "+
			"UNIQUE (order_line_id) là thứ chặn thật", n)
	}
}

// Hủy đơn phải ĐẢO hoa hồng, kèm lý do.
//
// affiliate.md quy tắc 5. Không có đường này thì hủy đơn để lại một khoản
// hoa hồng CHỜ ĐẾN HẠN cho doanh thu không còn tồn tại — và tới kỳ chi trả,
// nền tảng trả tiền cho một đơn đã hủy. Im lặng, và là tiền.
func TestHuyDonThiDaoHoaHongCreator(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	creatorID := ids.MustNew(ids.PrefixCreator).String()
	maDon, tong, _, _, dienThoai := a.donTraTruocCuaCreator(t, creatorID)

	whRes := a.goiWebhookThanhToan(t, "cong-tt", map[string]any{
		"event_id":   "evt_hoa_hong_dao_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_hoa_hong_dao_1",
			"amount":            tong,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": maDon},
		},
	}, biMatCongTT)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.phatEvent(t)

	if tt := a.trangThaiHoaHong(t, maDon); tt != "PENDING" {
		t.Fatalf("trước khi hủy: hoa hồng = %q, cần PENDING", tt)
	}

	// Hủy đơn qua ĐƯỜNG CỦA KHÁCH, không gọi service: đường ấy là đường duy
	// nhất phát `order.cancelled`.
	//
	// `reason` thuộc danh sách ĐÓNG năm giá trị của đặc tả, và khách vãng lai
	// chứng minh đơn là của mình bằng số điện thoại ở header.
	res := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/cancel",
		map[string]any{"reason": "CHANGED_MIND"},
		hopNhat(khoaIdem(), map[string]string{"X-Guest-Phone": dienThoai}))
	if res.code != http.StatusOK {
		t.Fatalf("hủy đơn: HTTP %d — %s", res.code, res.raw)
	}
	a.phatEvent(t)

	if tt := a.trangThaiHoaHong(t, maDon); tt != "REVERSED" {
		t.Fatalf("hủy đơn mà hoa hồng vẫn %q — nền tảng sẽ trả tiền cho một "+
			"đơn đã hủy", tt)
	}

	// Lý do BẮT BUỘC có, và phải là lý do THẬT của đơn.
	var lyDo string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT c.reversed_reason
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1`, maDon).Scan(&lyDo); err != nil {
		t.Fatalf("đọc lý do đảo: %v", err)
	}
	if lyDo == "" {
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
	a.phatEvent(t)

	if tt := a.trangThaiHoaHong(t, maDon); tt != "REVERSED" {
		t.Errorf("phát lại order.cancelled đổi trạng thái thành %q — đảo phải "+
			"IDEMPOTENT", tt)
	}
}

// ------------------------------------------------------------ tiện ích

// donTraTruocCuaCreator dựng một đơn TRẢ TRƯỚC từ nội dung của creator.
//
// TRẢ TRƯỚC chứ không COD: chỉ đơn trả trước đi qua `order.paid` ngay, còn
// COD phải chờ tới lúc giao. Bài test cần mốc tiền về ở giữa chuỗi.
func (a *apiTest) donTraTruocCuaCreator(
	t *testing.T, creatorID string,
) (maDon string, tong int64, foID, sellerID, dienThoai string) {
	t.Helper()

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
		"offer_id": maOffer, "quantity": 1,
		"source": map[string]any{"creator_id": creatorID},
	}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ kèm nguồn creator: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	// Số điện thoại RIÊNG cho mỗi đơn: nó là thứ duy nhất chứng minh đơn
	// thuộc về khách vãng lai này, và cả gói test dùng chung một database.
	dienThoai = "0900666" + ids.MustNew(ids.PrefixCreator).String()[24:27]
	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id":     maGio,
		"guest_email": emailMoi("hoahong"),
		"guest_phone": dienThoai,
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Hoa Hồng", "phone": dienThoai,
			"street_address": "7 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "CARD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ = don["id"].(string)
	if maDon == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}
	tongMap, _ := don["total"].(map[string]any)
	soTien, _ := tongMap["amount"].(float64)
	tong = int64(soTien)

	// Đơn thực hiện chỉ tồn tại sau khi `checkout.completed` được phát.
	a.phatEvent(t)
	foID, sellerID = a.donThucHienCuaDon(t, maDon)
	return maDon, tong, foID, sellerID, dienThoai
}

func (a *apiTest) demQuyCong(t *testing.T, maDon string) int {
	t.Helper()
	return a.demDong("attribution", "order_id = $1", maDon)
}

// trangThaiHoaHong trả trạng thái khoản hoa hồng DUY NHẤT của một đơn.
//
// Đỏ khi có nhiều hơn một: bài test dựng đơn MỘT dòng, nên hai khoản nghĩa
// là quy công đã chạy hai lần — thứ phải thấy ngay chứ không phải đọc dòng
// đầu rồi bỏ qua.
func (a *apiTest) trangThaiHoaHong(t *testing.T, maDon string) string {
	t.Helper()

	rows, err := a.db.Pool().Query(context.Background(), `
		SELECT c.status
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1`, maDon)
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
		t.Fatalf("đơn %s có %d khoản hoa hồng (%v), cần đúng 1", maDon, len(got), got)
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
func TestMoKhoaHoaHongTheoTungNhaBan(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	maDon, dsFO := a.donHaiNhaBanDaTraTien(t, "2nb")

	// Hoàn tất đơn thực hiện của MỘT nhà bán thôi.
	mot := dsFO[0]
	tokNB := a.taoTokenNhaBan(t, mot.sellerID)
	for _, buoc := range []string{"ship", "deliver"} {
		than := map[string]any{}
		if buoc == "ship" {
			than = map[string]any{
				"shipping_provider": "GHN", "tracking_number": "GHN-2NB-1",
			}
		}
		if r := a.call(http.MethodPost,
			"/api/v1/seller/fulfillment-orders/"+mot.id+"/"+buoc, than,
			hopNhat(khoaIdem(), map[string]string{
				"Authorization": "Bearer " + tokNB,
			})); r.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", buoc, r.code, r.raw)
		}
	}
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - $1::interval
		  WHERE id = $2`,
		(fulfillmentapp.ReturnWindow + time.Hour).String(), mot.id); err != nil {
		t.Fatalf("đẩy mốc giao: %v", err)
	}
	if n, err := a.mods.fulfillment.CompleteDelivered(ctx, 10); err != nil || n == 0 {
		t.Fatalf("hoàn tất đơn quá hạn: n=%d err=%v", n, err)
	}
	a.phatEvent(t)

	// ĐÚNG MỘT khoản mở, đúng của nhà bán đã hoàn tất.
	moTheoNhaBan := func(sellerID string) string {
		var tt string
		if err := a.db.Pool().QueryRow(ctx, `
			SELECT c.status
			  FROM commission_record c
			  JOIN attribution a ON a.id = c.attribution_id
			 WHERE a.order_id = $1 AND a.seller_id = $2`,
			maDon, sellerID).Scan(&tt); err != nil {
			t.Fatalf("đọc hoa hồng của nhà bán %s: %v", sellerID, err)
		}
		return tt
	}

	if tt := moTheoNhaBan(mot.sellerID); tt != "AVAILABLE" {
		t.Errorf("nhà bán đã hết hạn đổi trả mà hoa hồng vẫn %q", tt)
	}
	if tt := moTheoNhaBan(dsFO[1].sellerID); tt != "PENDING" {
		t.Fatalf("hoa hồng của nhà bán CHƯA giao xong đã %q — mở khóa bỏ qua "+
			"seller_id, nên hàng còn trong hạn đổi trả đã vào diện chi được", tt)
	}
}

// moPhienTuGio mở phiên thanh toán từ một giỏ và điền đủ địa chỉ, cách giao.
//
// Tách khỏi `donTraTruocCuaCreator` vì bài hai nhà bán tự dựng giỏ: nó cần
// hai offer, và ghép việc dựng giỏ vào cùng một tiện ích sẽ cho một hàm có
// hai chế độ.
func (a *apiTest) moPhienTuGio(t *testing.T, maGio, email, dienThoai string) string {
	t.Helper()

	res := a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": email, "guest_phone": dienThoai,
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Hoa Hồng", "phone": dienThoai,
			"street_address": "9 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem())

	return maPhien
}

// donThucHien là một đơn thực hiện trong bài test, kèm nhà bán của nó.
type donThucHien struct{ id, sellerID string }

// donHaiNhaBanDaTraTien dựng một đơn HAI nhà bán, cùng một creator, ĐÃ trả
// tiền và ĐÃ quy công.
//
// CÙNG một creator cho cả hai dòng có chủ ý: bài dùng nó kiểm ranh giới NHÀ
// BÁN, nên hai creator khác nhau sẽ làm lẫn hai nguyên nhân với nhau.
//
// `nhan` phân biệt mã idempotency của webhook giữa các bài: cả gói test dùng
// chung một database, nên hai bài gửi cùng `event_id` thì bài sau nhận lại
// phản hồi của bài trước và không có event nào được phát.
func (a *apiTest) donHaiNhaBanDaTraTien(
	t *testing.T, nhan string,
) (maDon string, dsFO []donThucHien) {
	t.Helper()
	ctx := context.Background()

	offerA, offerB := a.haiOfferKhacNhaBan(t)
	if offerA == "" || offerB == "" {
		t.Skip("không có đủ hai nhà bán bán được hàng")
	}

	creatorID := ids.MustNew(ids.PrefixCreator).String()
	for _, id := range []string{offerA, offerB} {
		r := a.call(http.MethodPost, "/api/v1/cart/items", map[string]any{
			"offer_id": id, "quantity": 1,
			"source": map[string]any{"creator_id": creatorID},
		}, khoaIdem())
		if r.code != http.StatusOK {
			t.Fatalf("thêm offer %s: HTTP %d — %s", id, r.code, r.raw)
		}
	}

	gio := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	c, _ := gio.body["cart"].(map[string]any)
	if nhom, _ := c["groups"].([]any); len(nhom) < 2 {
		t.Skipf("giỏ chỉ có %d nhà bán, cần 2", len(nhom))
	}
	maGio, _ := c["id"].(string)

	dienThoai := "09004" + ids.MustNew(ids.PrefixRequest).String()[25:30]
	maPhien := a.moPhienTuGio(t, maGio, emailMoi("hh"+nhan), dienThoai)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "CARD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ = don["id"].(string)
	tongMap, _ := don["total"].(map[string]any)
	soTien, _ := tongMap["amount"].(float64)

	// Phát TRƯỚC webhook: đơn thực hiện chỉ tồn tại sau `checkout.completed`.
	a.phatEvent(t)

	whRes := a.goiWebhookThanhToan(t, "cong-tt", map[string]any{
		"event_id":   "evt_hoa_hong_" + nhan,
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_hoa_hong_" + nhan,
			"amount":            int64(soTien),
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": maDon},
		},
	}, biMatCongTT)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.phatEvent(t)

	if n := a.demQuyCong(t, maDon); n != 2 {
		t.Fatalf("đơn hai nhà bán có %d bản quy công, cần 2 — quy công theo "+
			"TỪNG DÒNG, không theo cả đơn", n)
	}

	rows, err := a.db.Pool().Query(ctx,
		`SELECT id, seller_id FROM fulfillment_order WHERE order_id = $1
		  ORDER BY created_at`, maDon)
	if err != nil {
		t.Fatalf("đọc đơn thực hiện: %v", err)
	}
	for rows.Next() {
		var f donThucHien
		if err := rows.Scan(&f.id, &f.sellerID); err != nil {
			t.Fatalf("đọc đơn thực hiện: %v", err)
		}
		dsFO = append(dsFO, f)
	}
	rows.Close()
	if len(dsFO) != 2 {
		t.Fatalf("đơn tách thành %d đơn thực hiện, cần 2", len(dsFO))
	}

	return maDon, dsFO
}

// trangThaiHoaHongTheoNhaBan trả trạng thái khoản hoa hồng của một nhà bán
// trong một đơn.
func (a *apiTest) trangThaiHoaHongTheoNhaBan(
	t *testing.T, maDon, sellerID string,
) string {
	t.Helper()
	var tt string
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT c.status
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1 AND a.seller_id = $2`,
		maDon, sellerID).Scan(&tt); err != nil {
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
// nhà bán CÒN LẠI không được chạm tới. `DaoTheoDon` đảo theo `order_id`, nên
// dùng nó cho việc này sẽ xanh ở nửa đầu và sai ở nửa sau.
func TestHuyDonThucHienThiDaoHoaHongCuaDongDo(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.affiliate == nil {
		t.Skip("module affiliate cần kho lưu trữ postgres")
	}

	maDon, dsFO := a.donHaiNhaBanDaTraTien(t, "huy_fo")
	huy, conLai := dsFO[0], dsFO[1]

	for _, f := range dsFO {
		if tt := a.trangThaiHoaHongTheoNhaBan(t, maDon, f.sellerID); tt != "PENDING" {
			t.Fatalf("trước khi hủy: hoa hồng của %s là %q, cần PENDING",
				f.sellerID, tt)
		}
	}

	// Hủy qua API CỦA MODULE: không có route HTTP nào cho việc này, và đây
	// là đường production thật — `CancelFulfillment` là thứ phát
	// `fulfillment.cancelled`.
	if err := a.mods.fulfillment.CancelFulfillment(
		ctx, huy.sellerID, huy.id, "hết hàng",
	); err != nil {
		t.Fatalf("hủy đơn thực hiện: %v", err)
	}
	a.phatEvent(t)

	if tt := a.trangThaiHoaHongTheoNhaBan(t, maDon, huy.sellerID); tt != "REVERSED" {
		t.Fatalf("nhà bán hủy mà hoa hồng vẫn %q — khoản này sẽ đứng ở "+
			"CHỜ ĐẾN HẠN mãi mãi, vì đơn thực hiện bị hủy không bao giờ phát "+
			"fulfillment_order.completed (P3-85)", tt)
	}

	// VÀ dòng của nhà bán còn lại KHÔNG được chạm.
	if tt := a.trangThaiHoaHongTheoNhaBan(t, maDon, conLai.sellerID); tt != "PENDING" {
		t.Fatalf("hoa hồng của nhà bán VẪN ĐANG GIAO đã thành %q — đảo theo "+
			"order_id thay vì theo dòng đơn", tt)
	}

	// Lý do phải nêu rõ ĐƠN THỰC HIỆN nào: "đơn bị hủy" không tra ra được
	// chuyện đã xảy ra lúc đối soát.
	var lyDo string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT c.reversed_reason
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1 AND a.seller_id = $2`,
		maDon, huy.sellerID).Scan(&lyDo); err != nil {
		t.Fatalf("đọc lý do đảo: %v", err)
	}
	if !strings.Contains(lyDo, "đơn thực hiện") {
		t.Errorf("lý do đảo = %q — phải nêu rõ đơn thực hiện nào bị hủy", lyDo)
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
	a.phatEvent(t)

	if tt := a.trangThaiHoaHongTheoNhaBan(t, maDon, conLai.sellerID); tt != "PENDING" {
		t.Errorf("phát lại event đổi hoa hồng của nhà bán còn lại thành %q", tt)
	}
}
