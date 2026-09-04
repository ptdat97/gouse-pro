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

const goc = "http://localhost:8080"

func khoa() string {
	b := make([]byte, 13)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

type khach struct {
	cl  *http.Client
	gio string
	jar []*http.Cookie

	// them là header thêm vào mọi request — dùng cho token nhà bán.
	them map[string]string
}

func moKhach() *khach {
	return &khach{cl: &http.Client{Timeout: 20 * time.Second}}
}

func (k *khach) goi(method, duong string, than any) (int, map[string]any, time.Duration) {
	var r io.Reader
	if than != nil {
		b, _ := json.Marshal(than)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, goc+duong, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", khoa())
	for k2, v := range k.them {
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

func timOffer() string {
	res, err := http.Get(goc + "/api/v1/products?limit=10")
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var d struct {
		Data []struct{ ID string } `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&d)
	for _, p := range d.Data {
		r2, err := http.Get(goc + "/api/v1/products/" + p.ID + "/offers")
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

func thongKe(ten string, ds []time.Duration, loi int) {
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
		p(0.99).Round(time.Microsecond), ds[len(ds)-1].Round(time.Microsecond), loi)
}

func main() {
	songSong, _ := strconv.Atoi(os.Getenv("SONG_SONG"))
	if songSong == 0 {
		songSong = 20
	}
	moiNguoi, _ := strconv.Atoi(os.Getenv("MOI_NGUOI"))
	if moiNguoi == 0 {
		moiNguoi = 10
	}

	// KICH_BAN chọn phép đo. Mặc định giữ nguyên kịch bản cũ để lệnh ghi
	// trong docs/09-operations/do-tai.md vẫn chạy đúng như tài liệu nói.
	switch os.Getenv("KICH_BAN") {
	case "datdon":
		chayDatDon()
		return
	case "tonkho":
		chayTonKho()
		return
	}

	offer := timOffer()
	if offer == "" {
		fmt.Println("không tìm được offer bán được")
		os.Exit(1)
	}
	fmt.Printf("offer=%s  song song=%d  mỗi người=%d lượt\n\n", offer, songSong, moiNguoi)

	chay := func(ten string, chungGio bool) {
		// Mỗi kịch bản cần một giỏ có sẵn MỘT món để sửa số lượng.
		dungGio := func() (*khach, string) {
			k := moKhach()
			code, body, _ := k.goi(http.MethodPost, "/api/v1/cart/items",
				map[string]any{"offer_id": offer, "quantity": 1})
			if code != 200 {
				return nil, ""
			}
			gio, _ := body["cart"].(map[string]any)
			nhom, _ := gio["groups"].([]any)
			if len(nhom) == 0 {
				return nil, ""
			}
			g0, _ := nhom[0].(map[string]any)
			mon, _ := g0["items"].([]any)
			if len(mon) == 0 {
				return nil, ""
			}
			m0, _ := mon[0].(map[string]any)
			id, _ := m0["id"].(string)
			return k, id
		}

		var chungK *khach
		var chungMon string
		if chungGio {
			chungK, chungMon = dungGio()
			if chungMon == "" {
				fmt.Printf("%s: không dựng được giỏ chung\n", ten)
				return
			}
		}

		var mu sync.Mutex
		var ds []time.Duration
		var loi int
		var viDu string

		var batDau sync.WaitGroup
		var xong sync.WaitGroup
		batDau.Add(1)

		type viec struct {
			k   *khach
			mon string
		}
		cong := make([]viec, songSong)
		for i := range cong {
			if chungGio {
				k := moKhach()
				k.jar = append(k.jar, chungK.jar...)
				cong[i] = viec{k: k, mon: chungMon}
			} else {
				k, m := dungGio()
				if m == "" {
					fmt.Printf("%s: không dựng được giỏ riêng\n", ten)
					return
				}
				cong[i] = viec{k: k, mon: m}
			}
		}

		for i := 0; i < songSong; i++ {
			xong.Add(1)
			go func(v viec) {
				defer xong.Done()
				batDau.Wait()
				for j := 0; j < moiNguoi; j++ {
					// Đổi qua lại 1/2 để mỗi lượt là một thay đổi THẬT.
					q := 1 + j%2
					code, body, d := v.k.goi(http.MethodPatch,
						"/api/v1/cart/items/"+v.mon,
						map[string]any{"quantity": q})
					mu.Lock()
					if code == 200 {
						ds = append(ds, d)
					} else {
						loi++
						if viDu == "" {
							b, _ := json.Marshal(body)
							viDu = fmt.Sprintf("HTTP %d %s", code, string(b))
						}
					}
					mu.Unlock()
				}
			}(cong[i])
		}

		t0 := time.Now()
		batDau.Done()
		xong.Wait()
		tong := time.Since(t0)

		thongKe(ten, ds, loi)
		fmt.Printf("%-8s tổng %v  →  %.0f lượt/giây\n",
			"", tong.Round(time.Millisecond),
			float64(songSong*moiNguoi)/tong.Seconds())
		if viDu != "" {
			fmt.Printf("%-8s lỗi mẫu: %s\n", "", viDu[:min(len(viDu), 150)])
		}
		fmt.Println()
	}

	chay("riêng", false)
	chay("chung", true)
}
