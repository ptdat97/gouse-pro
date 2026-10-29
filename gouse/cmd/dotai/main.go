// Đo chi phí của khóa bi quan trên giỏ hàng.
//
// Hai kịch bản, KHÁC NHAU ĐÚNG MỘT BIẾN:
//
//	chung   N khách cùng SỬA một giỏ  → mọi lượt tranh cùng một dòng
//	riêng   N khách mỗi người một giỏ → không ai tranh với ai
//
// Dùng SỬA SỐ LƯỢNG chứ không phải THÊM MÓN: thêm món có trần 10 mỗi
// offer, nên kịch bản chung sẽ hỏng 191/200 lượt vì lỗi nghiệp vụ và phép
// đo thành đo tốc độ trả lỗi. Sửa số lượng đi qua ĐÚNG đường khóa dòng
// (MutateWithEvents) mà không có trần cộng dồn.
//
// Chênh lệch giữa hai con số CHÍNH LÀ giá của khóa. So với một hệ thống
// không có khóa là so nhầm: khi đó dữ liệu sai, và tốc độ của một hệ
// thống cho kết quả sai thì không có nghĩa gì.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

const original = "http://localhost:8080"

func key() string {
	b := make([]byte, 13)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

type client struct {
	cl   *http.Client
	cart string
	jar  []*http.Cookie

	// them là header thêm vào mọi request — dùng cho token nhà bán.
	add map[string]string
}

func newClient() *client {
	return &client{cl: &http.Client{Timeout: 20 * time.Second}}
}

func (k *client) parcel(method, path string, than any) (int, map[string]any, time.Duration) {
	var r io.Reader
	if than != nil {
		b, _ := json.Marshal(than)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, original+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key())
	for k2, v := range k.add {
		req.Header.Set(k2, v)
	}
	for _, c := range k.jar {
		req.AddCookie(c)
	}

	t0 := time.Now()
	res, err := k.cl.Do(req)
	d := time.Since(t0)
	if err != nil {
		return 0, nil, d
	}
	defer res.Body.Close()
	k.jar = append(k.jar, res.Cookies()...)

	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body, d
}

func pickOffer() string {
	res, err := http.Get(original + "/api/v1/products?limit=10")
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var d struct {
		Data []struct{ ID string } `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&d)
	for _, p := range d.Data {
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
				return x.ID
			}
		}
	}
	return ""
}

func thongKe(ten string, ds []time.Duration, failure int) {
	if len(ds) == 0 {
		fmt.Printf("%-8s KHÔNG có lượt nào thành công\n", ten)
		return
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	tong := time.Duration(0)
	for _, d := range ds {
		tong += d
	}
	p := func(q float64) time.Duration { return ds[int(float64(len(ds)-1)*q)] }
	fmt.Printf("%-8s n=%-4d  tb=%-8v  p50=%-8v  p95=%-8v  p99=%-8v  max=%-8v  lỗi=%d\n",
		ten, len(ds), (tong / time.Duration(len(ds))).Round(time.Microsecond),
		p(0.50).Round(time.Microsecond), p(0.95).Round(time.Microsecond),
		p(0.99).Round(time.Microsecond), ds[len(ds)-1].Round(time.Microsecond), failure)
}

func main() {
	songSong, _ := strconv.Atoi(os.Getenv("SONG_SONG"))
	if songSong == 0 {
		songSong = 20
	}
	perUser, _ := strconv.Atoi(os.Getenv("MOI_NGUOI"))
	if perUser == 0 {
		perUser = 10
	}

	// KICH_BAN chọn phép đo. Mặc định giữ nguyên kịch bản cũ để lệnh ghi
	// trong docs/09-operations/do-tai.md vẫn chạy đúng như tài liệu nói.
	switch os.Getenv("KICH_BAN") {
	case "datdon":
		runPlaceOrder()
		return
	case "tonkho":
		runStock()
		return
	}

	offer := pickOffer()
	if offer == "" {
		fmt.Println("không tìm được offer bán được")
		os.Exit(1)
	}
	fmt.Printf("offer=%s  song song=%d  mỗi người=%d lượt\n\n", offer, songSong, perUser)

	chay := func(ten string, sharedCart bool) {
		// Mỗi kịch bản cần một giỏ có sẵn MỘT món để sửa số lượng.
		buildCart := func() (*client, string) {
			k := newClient()
			code, body, _ := k.parcel(http.MethodPost, "/api/v1/cart/items",
				map[string]any{"offer_id": offer, "quantity": 1})
			if code != 200 {
				return nil, ""
			}
			cart, _ := body["cart"].(map[string]any)
			group, _ := cart["groups"].([]any)
			if len(group) == 0 {
				return nil, ""
			}
			g0, _ := group[0].(map[string]any)
			item, _ := g0["items"].([]any)
			if len(item) == 0 {
				return nil, ""
			}
			m0, _ := item[0].(map[string]any)
			id, _ := m0["id"].(string)
			return k, id
		}

		var sharedCustomer *client
		var sharedOffer string
		if sharedCart {
			sharedCustomer, sharedOffer = buildCart()
			if sharedOffer == "" {
				fmt.Printf("%s: không dựng được giỏ chung\n", ten)
				return
			}
		}

		var mu sync.Mutex
		var ds []time.Duration
		var failure int
		var sample string

		var start sync.WaitGroup
		var done sync.WaitGroup
		start.Add(1)

		type job struct {
			k    *client
			item string
		}
		sum := make([]job, songSong)
		for i := range sum {
			if sharedCart {
				k := newClient()
				k.jar = append(k.jar, sharedCustomer.jar...)
				sum[i] = job{k: k, item: sharedOffer}
			} else {
				k, m := buildCart()
				if m == "" {
					fmt.Printf("%s: không dựng được giỏ riêng\n", ten)
					return
				}
				sum[i] = job{k: k, item: m}
			}
		}

		for i := 0; i < songSong; i++ {
			done.Add(1)
			go func(v job) {
				defer done.Done()
				start.Wait()
				for j := 0; j < perUser; j++ {
					// Đổi qua lại 1/2 để mỗi lượt là một thay đổi THẬT.
					q := 1 + j%2
					code, body, d := v.k.parcel(http.MethodPatch,
						"/api/v1/cart/items/"+v.item,
						map[string]any{"quantity": q})
					mu.Lock()
					if code == 200 {
						ds = append(ds, d)
					} else {
						failure++
						if sample == "" {
							b, _ := json.Marshal(body)
							sample = fmt.Sprintf("HTTP %d %s", code, string(b))
						}
					}
					mu.Unlock()
				}
			}(sum[i])
		}

		t0 := time.Now()
		start.Done()
		done.Wait()
		tong := time.Since(t0)

		thongKe(ten, ds, failure)
		fmt.Printf("%-8s tổng %v  →  %.0f lượt/giây\n",
			"", tong.Round(time.Millisecond),
			float64(songSong*perUser)/tong.Seconds())
		if sample != "" {
			fmt.Printf("%-8s lỗi mẫu: %s\n", "", sample[:min(len(sample), 150)])
		}
		fmt.Println()
	}

	chay("riêng", false)
	chay("chung", true)
}
