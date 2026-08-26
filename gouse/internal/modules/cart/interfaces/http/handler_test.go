package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/application"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
	carthttp "github.com/fashion-commerce/platform/internal/modules/cart/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

// fakeRepo là kho lưu trữ trong bộ nhớ.
//
// # Vì sao KHÔNG dùng kho PostgreSQL thật ở đây
//
// Quy tắc R8 của archcheck cấm tầng interfaces import infrastructure — kể
// cả trong test. Đó không phải phiền toái: nó giữ cho test ở tầng này nói
// về CHUYỆN CỦA HTTP (định danh người gọi, hình dạng JSON, mã trạng thái)
// thay vì lẫn sang chuyện của database.
//
// Vòng ghi–đọc qua PostgreSQL được kiểm chứng ở cart/module_test.go, nơi
// import infrastructure là hợp lệ.
type fakeRepo struct{ carts map[ids.ID]*domain.Cart }

func newFakeRepo() *fakeRepo { return &fakeRepo{carts: map[ids.ID]*domain.Cart{}} }

var _ domain.Repository = (*fakeRepo)(nil)

func (r *fakeRepo) Save(_ context.Context, c *domain.Cart) error {
	r.carts[c.ID()] = c
	return nil
}

func (r *fakeRepo) SaveWithEvents(ctx context.Context, c *domain.Cart, fn domain.TxFunc) error {
	if err := r.Save(ctx, c); err != nil {
		return err
	}
	if fn == nil {
		return nil
	}
	return fn(ctx)
}

// MutateWithEvents chạy tuần tự trong bộ nhớ.
//
// Bản giả này KHÔNG mô phỏng khóa dòng và KHÔNG chứng minh gì về tranh
// chấp — nó chỉ giữ cho test tầng HTTP chạy được. Bất biến "hai lượt chồng
// nhau không ghi đè lẫn nhau" được kiểm ở internal/app/api_idempotency_test.go
// với PostgreSQL thật, vì đó là chỗ duy nhất kiểm được.
func (r *fakeRepo) MutateWithEvents(
	ctx context.Context, cartID ids.ID,
	apply func(*domain.Cart) error, fn domain.TxFunc,
) (*domain.Cart, error) {
	c, err := r.FindByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if err := apply(c); err != nil {
		return nil, err
	}
	if err := r.Save(ctx, c); err != nil {
		return nil, err
	}
	if fn != nil {
		if err := fn(ctx); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (r *fakeRepo) FindByID(_ context.Context, id ids.ID) (*domain.Cart, error) {
	c, ok := r.carts[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c, nil
}

func (r *fakeRepo) FindActiveByCustomer(
	_ context.Context, customerID ids.ID,
) (*domain.Cart, error) {
	for _, c := range r.carts {
		if c.IsActive() && c.CustomerID() == customerID {
			return c, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) FindActiveBySession(
	_ context.Context, sessionID string,
) (*domain.Cart, error) {
	for _, c := range r.carts {
		if c.IsActive() && c.SessionID() == sessionID {
			return c, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) Delete(_ context.Context, id ids.ID) error {
	delete(r.carts, id)
	return nil
}

// fakeLookup thay bốn module mà cart phụ thuộc.
//
// Cho phép mô tả thẳng tình huống cần thử — "hai seller, một món hết hàng"
// — mà không phải dựng marketplace, product, seller và inventory.
type fakeLookup struct{ data map[ids.ID]domain.SyncData }

func newFakeLookup() *fakeLookup {
	return &fakeLookup{data: map[ids.ID]domain.SyncData{}}
}

func (f *fakeLookup) LookupOffers(
	_ context.Context, offerIDs []ids.ID,
) (map[ids.ID]domain.SyncData, error) {
	out := map[ids.ID]domain.SyncData{}
	for _, id := range offerIDs {
		if d, ok := f.data[id]; ok {
			out[id] = d
		}
	}
	return out, nil
}

// offer khai báo một offer đang bán bình thường của một seller có tên.
func (f *fakeLookup) offer(sellerID ids.ID, sellerName string, price int64) ids.ID {
	offerID := ids.MustNew(ids.PrefixOffer)
	f.data[offerID] = domain.SyncData{
		OfferExists:       true,
		SellerActive:      true,
		IsSellable:        true,
		SKUID:             ids.MustNew(ids.PrefixSKU),
		SellerID:          sellerID,
		SellerName:        sellerName,
		ProductName:       "Áo sơ mi linen Oxford",
		UnitPrice:         money.MustNew(price, money.VND),
		AvailableQuantity: 100,
	}
	return offerID
}

// newHandler dựng handler đã bọc ResolveShopper, như lúc chạy thật.
func newHandler(t *testing.T) (http.Handler, *fakeLookup) {
	t.Helper()

	lookup := newFakeLookup()
	svc := application.NewService(application.Deps{
		Carts:  newFakeRepo(),
		Offers: lookup,
	})

	mux := http.NewServeMux()
	carthttp.NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	// resolver nil: mọi request là khách vãng lai, đúng luồng cần thử nhất.
	return httpserver.Chain(mux, httpserver.ResolveShopper(nil)), lookup
}

// call gửi một request và trả về response cùng phần thân đã giải mã.
func call(
	t *testing.T, h http.Handler, method, path, body string, cookie *http.Cookie,
) (*http.Response, map[string]any) {
	t.Helper()

	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	res := w.Result()

	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res, decoded
}

// session lấy cookie phiên vãng lai từ response đầu tiên.
func session(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == "shopper_session" {
			return c
		}
	}
	t.Fatal("response không cấp cookie phiên vãng lai")
	return nil
}

// TestGuestGetsEmptyCart kiểm chứng khách VÃNG LAI mở được giỏ.
//
// Đây là yêu cầu MVP: mua hàng không cần tài khoản. Trả 401 ở đây nghĩa là
// mọi khách chưa đăng ký bị chặn khỏi bước đầu tiên của việc mua hàng.
func TestGuestGetsEmptyCart(t *testing.T) {
	h, _ := newHandler(t)

	res, body := call(t, h, "GET", "/api/v1/cart", "", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200", res.StatusCode)
	}

	cart, ok := body["cart"].(map[string]any)
	if !ok {
		t.Fatalf("response không có khóa `cart`: %v", body)
	}

	// KHÔNG có `id`: từ 26/08 đường đọc không tạo giỏ nữa (PH-29), nên
	// khách chưa thêm món nào thì chưa có giỏ nào để đặt tên.
	//
	// Bịa một mã ra sẽ khiến giao diện tưởng nó gọi được các đường sửa
	// giỏ bằng mã đó, và nhận 404 ở lần thử đầu tiên.
	if id, co := cart["id"]; co && id != "" && id != nil {
		t.Errorf("giỏ chưa tồn tại mà có định danh %v", id)
	}

	// `groups` là trường BẮT BUỘC của đặc tả. Trả null thay vì mảng rỗng
	// làm client phải kiểm tra null trước mỗi vòng lặp.
	groups, ok := cart["groups"].([]any)
	if !ok {
		t.Fatalf("`groups` không phải mảng: %#v", cart["groups"])
	}
	if len(groups) != 0 {
		t.Errorf("giỏ mới có %d nhóm, muốn 0", len(groups))
	}
}

// TestCartIsBoundToSession kiểm chứng giỏ gắn với PHIÊN của người gọi.
//
// Hai khách vãng lai khác nhau phải nhận hai giỏ khác nhau. Nếu không,
// người này thấy hàng người kia đã thêm.
func TestCartIsBoundToSession(t *testing.T) {
	h, lookup := newHandler(t)
	offerA := lookup.offer(ids.MustNew(ids.PrefixSeller), "Cửa hàng ABC", 299_000)

	// Giỏ chỉ tồn tại sau khi THÊM MÓN — đường đọc không tạo gì (PH-29).
	// Nên bài này dựng giỏ bằng đúng cách khách dựng ra nó.
	first, addA := call(t, h, "POST", "/api/v1/cart/items",
		`{"offer_id":"`+offerA.String()+`","quantity":1}`, nil)
	cookie := session(t, first)
	idA := addA["cart"].(map[string]any)["id"]
	if idA == nil || idA == "" {
		t.Fatalf("thêm món không trả giỏ: %v", addA)
	}

	// Cùng cookie → cùng giỏ, và THẤY món vừa thêm.
	_, bodyB := call(t, h, "GET", "/api/v1/cart", "", cookie)
	cartB := bodyB["cart"].(map[string]any)
	if cartB["id"] != idA {
		t.Errorf("cùng phiên nhận hai giỏ khác nhau: %v và %v", idA, cartB["id"])
	}
	if n := len(cartB["groups"].([]any)); n == 0 {
		t.Error("cùng phiên mà không thấy món vừa thêm")
	}

	// Phiên KHÁC → không thấy gì của phiên trước.
	//
	// Đây mới là điều bài test này bảo vệ: người này KHÔNG được thấy hàng
	// người kia đã thêm. Trước đây nó kiểm gián tiếp bằng cách so hai mã
	// giỏ; nay kiểm thẳng vào thứ đáng lo.
	_, bodyC := call(t, h, "GET", "/api/v1/cart", "", nil)
	cartC := bodyC["cart"].(map[string]any)
	if cartC["id"] == idA {
		t.Error("hai phiên khác nhau dùng chung một giỏ")
	}
	if n := len(cartC["groups"].([]any)); n != 0 {
		t.Errorf("phiên mới thấy %d nhóm hàng của phiên khác", n)
	}
}

// TestItemsGroupedBySeller kiểm chứng giỏ trả về NHÓM THEO SELLER.
//
// Đặc tả yêu cầu điều này vì khách cần biết hàng đến từ đâu: hai nguồn
// hàng nghĩa là hai gói giao với hai thời gian khác nhau.
func TestItemsGroupedBySeller(t *testing.T) {
	h, lookup := newHandler(t)

	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)
	offerA := lookup.offer(sellerA, "Cửa hàng ABC", 299_000)
	offerB := lookup.offer(sellerB, "Xưởng may XYZ", 150_000)

	res, _ := call(t, h, "GET", "/api/v1/cart", "", nil)
	cookie := session(t, res)

	add := func(offerID ids.ID, qty int) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"offer_id": offerID.String(), "quantity": qty,
		})
		res, decoded := call(t, h, "POST", "/api/v1/cart/items", string(body), cookie)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("thêm món: mã trạng thái = %d, thân = %v", res.StatusCode, decoded)
		}
		return decoded
	}

	add(offerA, 2)
	body := add(offerB, 1)

	groups := body["cart"].(map[string]any)["groups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("có %d nhóm, muốn 2 (mỗi seller một nhóm)", len(groups))
	}

	// Tên seller phải đi cùng nhóm: đặc tả bắt buộc `name` trong SellerRef,
	// và "Cửa hàng ABC" là thứ khách đọc được, khác hẳn `sel_01M0...`.
	names := map[string]bool{}
	for _, g := range groups {
		seller := g.(map[string]any)["seller"].(map[string]any)
		names[seller["name"].(string)] = true
	}
	for _, want := range []string{"Cửa hàng ABC", "Xưởng may XYZ"} {
		if !names[want] {
			t.Errorf("thiếu tên seller %q trong response: %v", want, names)
		}
	}

	// Tổng của giỏ = 2×299.000 + 1×150.000.
	subtotal := body["cart"].(map[string]any)["subtotal"].(map[string]any)
	if got := subtotal["amount"].(float64); got != 748_000 {
		t.Errorf("tổng tiền = %v, muốn 748000", got)
	}
}

// TestUnavailableItemStaysButIsNotCharged kiểm chứng QUY TẮC 6.
//
// Món không còn bán được ĐÁNH DẤU chứ không xóa — khách nhớ đã thêm nó và
// sẽ hoang mang nếu nó biến mất. Nhưng nó KHÔNG được tính vào tổng tiền:
// hiện một con số bao gồm hàng đã hết là hứa hẹn thứ không giao được.
func TestUnavailableItemStaysButIsNotCharged(t *testing.T) {
	h, lookup := newHandler(t)

	seller := ids.MustNew(ids.PrefixSeller)
	offerID := lookup.offer(seller, "Cửa hàng ABC", 299_000)

	res, _ := call(t, h, "GET", "/api/v1/cart", "", nil)
	cookie := session(t, res)

	body, _ := json.Marshal(map[string]any{"offer_id": offerID.String(), "quantity": 1})
	if res, decoded := call(t, h, "POST", "/api/v1/cart/items", string(body), cookie); res.StatusCode != http.StatusOK {
		t.Fatalf("thêm món: mã trạng thái = %d, thân = %v", res.StatusCode, decoded)
	}

	// Seller bị đình chỉ SAU khi khách đã thêm hàng vào giỏ.
	d := lookup.data[offerID]
	d.SellerActive = false
	lookup.data[offerID] = d

	res, decoded := call(t, h, "GET", "/api/v1/cart", "", cookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mã trạng thái = %d, muốn 200", res.StatusCode)
	}

	cart := decoded["cart"].(map[string]any)
	groups := cart["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("món biến mất khỏi giỏ: có %d nhóm, muốn 1", len(groups))
	}

	items := groups[0].(map[string]any)["items"].([]any)
	if got := items[0].(map[string]any)["availability"]; got != "UNAVAILABLE" {
		t.Errorf("availability = %v, muốn UNAVAILABLE", got)
	}
	if got := cart["subtotal"].(map[string]any)["amount"].(float64); got != 0 {
		t.Errorf("tổng tiền = %v, muốn 0 — món không bán được vẫn bị tính tiền", got)
	}
}

// TestPriceIsRefreshedOnRead kiểm chứng giỏ trả GIÁ HIỆN TẠI.
//
// Giỏ là Ý ĐỊNH mua, không phải hợp đồng: giá phải phản ánh thực tế. Bỏ
// bước đồng bộ thì khách thấy giá cũ ở giỏ rồi bị tính giá khác ở bước
// thanh toán — đúng thứ module này hứa sẽ không xảy ra.
func TestPriceIsRefreshedOnRead(t *testing.T) {
	h, lookup := newHandler(t)

	seller := ids.MustNew(ids.PrefixSeller)
	offerID := lookup.offer(seller, "Cửa hàng ABC", 299_000)

	res, _ := call(t, h, "GET", "/api/v1/cart", "", nil)
	cookie := session(t, res)

	body, _ := json.Marshal(map[string]any{"offer_id": offerID.String(), "quantity": 1})
	call(t, h, "POST", "/api/v1/cart/items", string(body), cookie)

	// Seller giảm giá.
	d := lookup.data[offerID]
	d.UnitPrice = money.MustNew(199_000, money.VND)
	lookup.data[offerID] = d

	_, decoded := call(t, h, "GET", "/api/v1/cart", "", cookie)
	cart := decoded["cart"].(map[string]any)
	if got := cart["subtotal"].(map[string]any)["amount"].(float64); got != 199_000 {
		t.Errorf("tổng tiền = %v, muốn 199000 — giỏ trả giá cũ", got)
	}
}

// TestQuantityMustBePositive kiểm chứng handler chặn số lượng vô nghĩa.
func TestQuantityMustBePositive(t *testing.T) {
	h, lookup := newHandler(t)
	offerID := lookup.offer(ids.MustNew(ids.PrefixSeller), "Cửa hàng ABC", 299_000)

	res, _ := call(t, h, "GET", "/api/v1/cart", "", nil)
	cookie := session(t, res)

	body, _ := json.Marshal(map[string]any{"offer_id": offerID.String(), "quantity": 0})
	res, _ = call(t, h, "POST", "/api/v1/cart/items", string(body), cookie)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400", res.StatusCode)
	}
}

// TestMissingShopperFailsClosed kiểm chứng handler TỪ CHỐI khi thiếu
// middleware ResolveShopper.
//
// Không có danh tính thì mọi request sẽ dùng chung một giỏ "rỗng danh
// tính" — nghĩa là khách này thấy hàng của khách kia. Thà trả 500 còn hơn.
func TestMissingShopperFailsClosed(t *testing.T) {
	svc := application.NewService(application.Deps{
		Carts:  newFakeRepo(),
		Offers: newFakeLookup(),
	})

	// KHÔNG bọc ResolveShopper — đây chính là tình huống cần thử.
	mux := http.NewServeMux()
	carthttp.NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	res, _ := call(t, mux, "GET", "/api/v1/cart", "", nil)
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("mã trạng thái = %d, muốn 500", res.StatusCode)
	}
}

// TestUpdateAndRemoveItem kiểm chứng hai thao tác sửa giỏ còn lại.
//
// Cùng một test vì chúng là hai nửa của một hành vi: khách đổi ý về số
// lượng, rồi đổi ý về cả món hàng.
func TestUpdateAndRemoveItem(t *testing.T) {
	h, lookup := newHandler(t)
	offerID := lookup.offer(ids.MustNew(ids.PrefixSeller), "Cửa hàng ABC", 299_000)

	res, _ := call(t, h, "GET", "/api/v1/cart", "", nil)
	cookie := session(t, res)

	body, _ := json.Marshal(map[string]any{"offer_id": offerID.String(), "quantity": 1})
	_, added := call(t, h, "POST", "/api/v1/cart/items", string(body), cookie)
	itemID := added["cart"].(map[string]any)["groups"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["id"].(string)

	// Đổi số lượng thành 3 → tổng phải là 897.000.
	res, updated := call(t, h, "PATCH", "/api/v1/cart/items/"+itemID,
		`{"quantity":3}`, cookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cập nhật số lượng: mã trạng thái = %d, thân = %v", res.StatusCode, updated)
	}
	subtotal := updated["cart"].(map[string]any)["subtotal"].(map[string]any)
	if got := subtotal["amount"].(float64); got != 897_000 {
		t.Errorf("tổng tiền sau cập nhật = %v, muốn 897000", got)
	}

	// Xóa món → giỏ rỗng, không còn nhóm nào.
	res, removed := call(t, h, "DELETE", "/api/v1/cart/items/"+itemID, "", cookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("xóa món: mã trạng thái = %d, thân = %v", res.StatusCode, removed)
	}
	if groups := removed["cart"].(map[string]any)["groups"].([]any); len(groups) != 0 {
		t.Errorf("còn %d nhóm sau khi xóa món cuối cùng, muốn 0", len(groups))
	}
}

// TestRemoveItemTraGiaHienTai kiểm chứng response của DELETE cũng ĐÃ ĐỒNG BỘ.
//
// RemoveItem ở tầng application KHÔNG tự đồng bộ (khác AddItem và
// UpdateQuantity), nên handler phải làm. Bỏ sót thì xóa một món xong, các
// món còn lại hiện giá cũ — và khách chỉ phát hiện ở bước thanh toán.
func TestRemoveItemTraGiaHienTai(t *testing.T) {
	h, lookup := newHandler(t)
	seller := ids.MustNew(ids.PrefixSeller)
	giu := lookup.offer(seller, "Cửa hàng ABC", 299_000)
	bo := lookup.offer(seller, "Cửa hàng ABC", 150_000)

	res, _ := call(t, h, "GET", "/api/v1/cart", "", nil)
	cookie := session(t, res)

	addOne := func(offerID ids.ID) string {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"offer_id": offerID.String(), "quantity": 1})
		_, decoded := call(t, h, "POST", "/api/v1/cart/items", string(body), cookie)
		items := decoded["cart"].(map[string]any)["groups"].([]any)[0].(map[string]any)["items"].([]any)
		for _, it := range items {
			m := it.(map[string]any)
			if m["offer_id"] == offerID.String() {
				return m["id"].(string)
			}
		}
		t.Fatalf("không tìm thấy món vừa thêm cho offer %s", offerID)
		return ""
	}

	addOne(giu)
	boID := addOne(bo)

	// Seller giảm giá món ĐƯỢC GIỮ LẠI, ngay trước khi khách xóa món kia.
	d := lookup.data[giu]
	d.UnitPrice = money.MustNew(199_000, money.VND)
	lookup.data[giu] = d

	_, decoded := call(t, h, "DELETE", "/api/v1/cart/items/"+boID, "", cookie)
	subtotal := decoded["cart"].(map[string]any)["subtotal"].(map[string]any)
	if got := subtotal["amount"].(float64); got != 199_000 {
		t.Errorf("tổng tiền = %v, muốn 199000 — response của DELETE trả giá cũ", got)
	}
}
