package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Đo TOÀN BỘ luồng đặt hàng dưới tải — mục "chưa đo" số 1 của
// docs/09-operations/do-tai.md, và là PH-15.
//
// # Đo TỪNG BƯỚC, không chỉ tổng
//
// Tổng thời gian đặt một đơn không nói được nên sửa ở đâu. Chuỗi này có
// năm bước qua bốn module, và chúng khác hẳn nhau về bản chất: thêm giỏ
// khóa một dòng, hoàn tất giữ hàng rồi tạo đơn rồi ghi outbox. Biết bước
// nào chậm là toàn bộ giá trị của phép đo.
//
// # Vì sao TRẢI trên nhiều offer
//
// Dồn hết vào một offer thì tồn kho cạn giữa chừng và phép đo biến thành
// đo tốc độ trả lỗi "hết hàng" — đúng cái bẫy mà kịch bản khóa giỏ đã mắc
// ở lần chạy đầu (xem chú thích đầu main.go).
//
// Hết hàng vẫn được đếm RIÊNG chứ không gộp vào lỗi: nó là kết quả nghiệp
// vụ hợp lệ. Nhưng nếu nó chiếm phần lớn thì con số thông lượng vô nghĩa,
// nên chương trình nói thẳng điều đó ra.

type step struct {
	ten string
	ds  []time.Duration
}

type orderResult struct {
	steps      []step
	tong       []time.Duration
	outOfStock int
	failure    int
	sample     string
}

// pickOffers lấy tối đa `can` offer bán được, mỗi sản phẩm một cái.
func pickOffers(can int) []string {
	res, err := http.Get(original + "/api/v1/products?limit=100")
	if err != nil {
		return nil
	}
	defer res.Body.Close()

	var d struct {
		Data []struct{ ID string } `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&d)

	var ra []string
	for _, p := range d.Data {
		if len(ra) >= can {
			break
		}
		r2, err := http.Get(original + "/api/v1/products/" + p.ID + "/offers")
		if err != nil {
			continue
		}
		var o struct {
			Data []struct {
				ID         string `json:"id"`
				IsSellable bool   `json:"is_sellable"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r2.Body).Decode(&o)
		r2.Body.Close()
		for _, x := range o.Data {
			if x.IsSellable {
				ra = append(ra, x.ID)
				break
			}
		}
	}
	return ra
}

// placeOneOrder chạy trọn chuỗi và trả thời gian từng bước.
//
// Trả nil khi hỏng giữa chừng: một đơn dở dang không được tính vào phân vị
// độ trễ, vì nó không phải một lần đặt hàng thành công.
func placeOneOrder(offer string) (map[string]time.Duration, string, string) {
	k := newClient()
	d := map[string]time.Duration{}

	code, body, t := k.parcel(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": offer, "quantity": 1})
	d["1.thêm giỏ"] = t
	if code != http.StatusOK {
		return nil, description(code, body), "1.thêm giỏ"
	}
	cart, _ := body["cart"].(map[string]any)
	cartID, _ := cart["id"].(string)

	code, body, t = k.parcel(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id":     cartID,
		"guest_email": "tai-" + key() + "@dotai.local",
		"guest_phone": "0900000001",
	})
	d["2.mở phiên"] = t
	if code != http.StatusCreated && code != http.StatusOK {
		return nil, description(code, body), "2.mở phiên"
	}
	checkoutID, _ := body["id"].(string)

	code, body, t = k.parcel(http.MethodPatch,
		"/api/v1/checkout/"+checkoutID+"/shipping-address", map[string]any{
			"recipient_name": "Khách Đo Tải", "phone": "0900000001",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		})
	d["3.địa chỉ"] = t
	if code >= 400 {
		return nil, description(code, body), "3.địa chỉ"
	}

	code, body, t = k.parcel(http.MethodPatch,
		"/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"})
	d["4.giao hàng"] = t
	if code >= 400 {
		return nil, description(code, body), "4.giao hàng"
	}

	code, body, t = k.parcel(http.MethodPost,
		"/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"})
	d["5.hoàn tất"] = t
	if code >= 400 {
		return nil, description(code, body), "5.hoàn tất"
	}
	return d, "", ""
}

func description(code int, body map[string]any) string {
	b, _ := json.Marshal(body)
	s := string(b)
	if len(s) > 160 {
		s = s[:160]
	}
	return fmt.Sprintf("HTTP %d %s", code, s)
}

// isOutOfStock phân biệt kết quả NGHIỆP VỤ với lỗi hệ thống.
//
// Khớp theo MÃ LỖI mà API thật trả về, không theo câu chữ tiếng Việt: câu
// chữ đổi được mà không ai nghĩ tới phép đo này, và khi đó lỗi nghiệp vụ
// sẽ lặng lẽ bị đếm thành lỗi hệ thống.
//
// Bản đầu khớp "OUT_OF_STOCK" — một mã KHÔNG tồn tại trong hệ thống này —
// nên mọi lượt hết hàng đều bị đếm nhầm. Lấy mã từ lần chạy thật.
func isOutOfStock(open string) bool {
	return contains(open, "INSUFFICIENT_INVENTORY")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func runPlaceOrder() {
	songSong, _ := strconv.Atoi(os.Getenv("SONG_SONG"))
	if songSong == 0 {
		songSong = 20
	}
	perUser, _ := strconv.Atoi(os.Getenv("MOI_NGUOI"))
	if perUser == 0 {
		perUser = 5
	}

	offers := pickOffers(songSong)
	if len(offers) == 0 {
		fmt.Println("không tìm được offer bán được")
		os.Exit(1)
	}
	fmt.Printf("kịch bản=đặt đơn  offer=%d  song song=%d  mỗi người=%d đơn\n\n",
		len(offers), songSong, perUser)

	var mu sync.Mutex
	kq := orderResult{}
	byStep := map[string][]time.Duration{}
	failedAtStep := map[string]int{}
	outOfStockAtStep := map[string]int{}

	var start, done sync.WaitGroup
	start.Add(1)

	for i := 0; i < songSong; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			// Mỗi luồng bám MỘT offer riêng: trải tải ra nhiều SKU để
			// không cạn kho, và giữ tranh chấp ở mức thật.
			offer := offers[i%len(offers)]
			for j := 0; j < perUser; j++ {
				t0 := time.Now()
				d, open, failedStep := placeOneOrder(offer)
				tong := time.Since(t0)

				mu.Lock()
				switch {
				case d != nil:
					kq.tong = append(kq.tong, tong)
					for ten, x := range d {
						byStep[ten] = append(byStep[ten], x)
					}
				case isOutOfStock(open):
					kq.outOfStock++
					outOfStockAtStep[failedStep]++
				default:
					kq.failure++
					failedAtStep[failedStep]++
					if kq.sample == "" {
						kq.sample = open
					}
				}
				mu.Unlock()
			}
		}(i)
	}

	t0 := time.Now()
	start.Done()
	done.Wait()
	tong := time.Since(t0)

	var ten []string
	for k := range byStep {
		ten = append(ten, k)
	}
	sort.Strings(ten)
	for _, b := range ten {
		thongKe(b, byStep[b], 0)
	}
	fmt.Println()
	thongKe("TỔNG/đơn", kq.tong, kq.failure)

	succeeded := len(kq.tong)
	fmt.Printf("%-8s %v  →  %.1f đơn/giây  ·  hết hàng=%d  lỗi=%d\n",
		"", tong.Round(time.Millisecond),
		float64(succeeded)/tong.Seconds(), kq.outOfStock, kq.failure)

	// Nói thẳng khi phép đo không còn nghĩa, thay vì in một con số đẹp.
	if succeeded == 0 {
		fmt.Println("\n!! KHÔNG đơn nào thành công — con số trên vô nghĩa")
	} else if kq.outOfStock > succeeded {
		fmt.Printf("\n!! hết hàng (%d) NHIỀU HƠN đơn thành công (%d) — "+
			"đây đang là phép đo tốc độ trả lỗi, không phải đo đặt hàng.\n"+
			"   Nạp thêm tồn kho hoặc giảm MOI_NGUOI rồi đo lại.\n",
			kq.outOfStock, succeeded)
	}
	// Bước nào hỏng QUAN TRỌNG hơn tổng số lỗi: nó chỉ thẳng chỗ cần xem.
	if len(outOfStockAtStep) > 0 {
		fmt.Printf("%-8s hết hàng theo bước: %v\n", "", outOfStockAtStep)
	}
	if len(failedAtStep) > 0 {
		fmt.Printf("%-8s lỗi theo bước: %v\n", "", failedAtStep)
	}
	if kq.sample != "" {
		fmt.Printf("%-8s lỗi mẫu: %s\n", "", kq.sample)
	}
}
