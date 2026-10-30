package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Đo nhà bán cập nhật tồn kho ĐỒNG THỜI với khách đang mua — PH-17.
//
// # Vì sao trộn hai phía, không đo riêng nhà bán
//
// N nhà bán tranh nhau sửa một dòng là tình huống KHÔNG có thật: một gian
// hàng có một người quản kho, và họ không bấm Lưu hai trăm lần một giây.
//
// Tình huống thật là hai phía KHÁC NHAU chạm cùng một dòng tồn kho: nhân
// viên kho kiểm kê lúc mười giờ sáng, đúng lúc khách đang đặt hàng. Cả
// hai đều đi qua khóa lạc quan, và câu hỏi là bên nào thua, thua thế nào.
//
// # Điều cần khẳng định
//
//	khách  KHÔNG được nhận 5xx, và không bị từ chối oan khi kho còn hàng
//	seller KHÔNG được nhận 5xx; thua tranh chấp thì phải là 409
//
// 5xx ở đây sẽ là lỗi thật: xung đột phiên bản là chuyện bình thường dưới
// tải, và trả 500 cho nó khiến người gọi không thử lại — trong khi thử
// lại chính là việc đúng.

func envStr(ten, fallback string) string {
	if v := os.Getenv(ten); v != "" {
		return v
	}
	return fallback
}

type countByCode struct {
	mu sync.Mutex
	m  map[int]int
	ds []time.Duration
}

func (d *countByCode) write(code int, t time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = map[int]int{}
	}
	d.m[code]++
	if code < 400 {
		d.ds = append(d.ds, t)
	}
}

func (d *countByCode) hasMachineError() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for code := range d.m {
		if code >= 500 || code == 0 {
			return true
		}
	}
	return false
}

func runStock() {
	sku := envStr("SKU", "")
	offer := envStr("OFFER", "")
	tok := envStr("SELLER_TOKEN", "")
	if sku == "" || offer == "" || tok == "" {
		fmt.Println("cần SKU, OFFER và SELLER_TOKEN")
		os.Exit(1)
	}

	concurrency, _ := strconv.Atoi(os.Getenv("SONG_SONG"))
	if concurrency == 0 {
		concurrency = 50
	}
	// SELLER_SONG_SONG=0 nghĩa là KHÔNG có nhà bán nào — kịch bản đối
	// chứng. Phải phân biệt "đặt là 0" với "không đặt", nếu không thì
	// không dựng được phép so khác nhau ĐÚNG MỘT BIẾN.
	sellerN := 5
	if v := os.Getenv("SELLER_SONG_SONG"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			fmt.Println("SELLER_SONG_SONG phải là số")
			os.Exit(1)
		}
		sellerN = n
	}
	perUser, _ := strconv.Atoi(os.Getenv("MOI_NGUOI"))
	if perUser == 0 {
		perUser = 5
	}

	fmt.Printf("kịch bản=tồn kho trộn  sku=%s\n", sku)
	fmt.Printf("khách=%d×%d  nhà bán=%d×%d  (cùng MỘT dòng tồn kho)\n\n",
		concurrency, perUser, sellerN, perUser)

	var client, seller countByCode
	var start, done sync.WaitGroup
	start.Add(1)

	// Phía KHÁCH: mở phiên thanh toán, tức là GIỮ HÀNG.
	//
	// Chỉ chạy tới bước giữ hàng là đủ: đó là bước duy nhất chạm dòng tồn
	// kho, và kéo dài chuỗi thêm sẽ pha loãng thứ đang đo.
	for i := 0; i < concurrency; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			for j := 0; j < perUser; j++ {
				k := newClient()
				code, body, _ := k.parcel(http.MethodPost, "/api/v1/cart/items",
					map[string]any{"offer_id": offer, "quantity": 1})
				if code != http.StatusOK {
					client.write(code, 0)
					continue
				}
				cart, _ := body["cart"].(map[string]any)
				cartID, _ := cart["id"].(string)

				t0 := time.Now()
				code, _, _ = k.parcel(http.MethodPost, "/api/v1/checkout",
					map[string]any{
						"cart_id":     cartID,
						"guest_email": "tk-" + key() + "@dotai.local",
						"guest_phone": "0900000002",
					})
				client.write(code, time.Since(t0))
			}
		}()
	}

	// Phía NHÀ BÁN: kiểm kê lại, con số tuyệt đối.
	for i := 0; i < sellerN; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			for j := 0; j < perUser; j++ {
				k := newClient()
				k.add = map[string]string{"Authorization": "Bearer " + tok}
				// Đổi qua lại hai con số để mỗi lượt là thay đổi THẬT:
				// đặt đúng số đang có là việc không-làm-gì, và đo nó thì
				// đo nhầm một đường không ghi database.
				q := 40000 + (i*7+j)%2
				t0 := time.Now()
				code, _, _ := k.parcel(http.MethodPut, "/api/v1/seller/inventory/"+sku,
					map[string]any{
						"quantity_available": q,
						"reason":             "kiểm kê định kỳ trong lúc đo tải",
					})
				seller.write(code, time.Since(t0))
			}
		}(i)
	}

	t0 := time.Now()
	start.Done()
	done.Wait()
	tong := time.Since(t0)

	printResult("khách  ", &client)
	if sellerN > 0 {
		printResult("nhà bán", &seller)
	} else {
		fmt.Printf("%-8s (đối chứng: không có nhà bán nào ghi)\n", "nhà bán")
	}
	fmt.Printf("\ntổng %v\n", tong.Round(time.Millisecond))

	if client.hasMachineError() || seller.hasMachineError() {
		fmt.Println("\n!! CÓ mã 5xx — xung đột phiên bản KHÔNG được ra 5xx")
	}
}

func printResult(ten string, d *countByCode) {
	d.mu.Lock()
	m := map[int]int{}
	for k, v := range d.m {
		m[k] = v
	}
	ds := append([]time.Duration(nil), d.ds...)
	d.mu.Unlock()

	b, _ := json.Marshal(m)
	report(ten, ds, 0)
	fmt.Printf("%-8s mã trạng thái: %s\n", "", string(b))
}
