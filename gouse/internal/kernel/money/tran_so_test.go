package money_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// TestPhanBoSoLonVanDungTyLe.
//
// # Vì sao "tổng khớp" KHÔNG đủ để kiểm phép chia tiền
//
// Bản đầu của `Allocate` nhân `amount * r` bằng int64. Chia 20 tỷ theo tỷ
// lệ 19 tỷ : 1 tỷ cho tích 3,8e20, vượt trần int64 (9,22e18).
//
// Kết quả: 9,78 tỷ / 10,22 tỷ thay vì 19 tỷ / 1 tỷ — gần 50/50 thay vì
// 95/5. Nhưng TỔNG vẫn đúng 20 tỷ, vì phần dư được rải bù.
//
// Nghĩa là bài test hiển nhiên nhất — "tổng các phần bằng tổng ban đầu" —
// VẪN XANH trong khi tiền chia sai gần gấp đôi. Đó là kiểu hỏng tệ nhất
// với tiền: sai âm thầm, chỉ lộ khi ai đó đối chiếu từng dòng.
//
// Hậu quả thật: `promotion.AllocateToLines` chia giảm giá xuống từng dòng
// hàng, và số tiền đó là căn cứ hoàn tiền khi khách trả MỘT món. Chia sai
// nghĩa là hoàn sai.
func TestPhanBoSoLonVanDungTyLe(t *testing.T) {
	ca := []struct {
		ten    string
		soTien int64
		tyLe   []int64
		mong   []int64
	}{
		{
			"số nhỏ — vốn đã đúng",
			1_000_000, []int64{700_000, 300_000},
			[]int64{700_000, 300_000},
		},
		{
			"20 tỷ, tỷ lệ 19:1 — tích 3,8e20 tràn int64",
			20_000_000_000, []int64{19_000_000_000, 1_000_000_000},
			[]int64{19_000_000_000, 1_000_000_000},
		},
		{
			"100 tỷ chia ba dòng lớn",
			100_000_000_000,
			[]int64{50_000_000_000, 30_000_000_000, 20_000_000_000},
			[]int64{50_000_000_000, 30_000_000_000, 20_000_000_000},
		},
	}

	for _, c := range ca {
		t.Run(c.ten, func(t *testing.T) {
			m, err := money.New(c.soTien, "VND")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			phan, err := m.Allocate(c.tyLe)
			if err != nil {
				t.Fatalf("Allocate: %v", err)
			}
			if len(phan) != len(c.mong) {
				t.Fatalf("có %d phần, cần %d", len(phan), len(c.mong))
			}

			var tong int64
			for i, p := range phan {
				tong += p.Amount()
				if p.Amount() != c.mong[i] {
					t.Errorf("phần %d = %d, cần %d — tỷ lệ SAI dù tổng có "+
						"thể vẫn khớp", i, p.Amount(), c.mong[i])
				}
			}
			// Tổng vẫn phải khớp — nhưng đó là điều kiện CẦN, không đủ.
			if tong != c.soTien {
				t.Errorf("tổng = %d, cần %d", tong, c.soTien)
			}
		})
	}
}

// TestPhanBoKhongTreo.
//
// Ở một tỷ lệ khác, tràn số làm `share` ÂM, nên `allocated` âm khổng lồ và
// phần dư `amount - allocated` thành gần 9e18. Vòng rải phần dư chạy tới
// chừng ấy lần — tiến trình TREO CỨNG, không phải trả kết quả sai.
//
// Một request treo giữ luôn kết nối database của nó, nên vài request như
// vậy là đủ làm cạn pool và kéo sập cả API.
func TestPhanBoKhongTreo(t *testing.T) {
	xong := make(chan struct{})
	go func() {
		defer close(xong)
		m, err := money.New(math.MaxInt64/4, "VND")
		if err != nil {
			return
		}
		_, _ = m.Allocate([]int64{math.MaxInt64 / 4, 1})
	}()

	select {
	case <-xong:
	case <-time.After(10 * time.Second):
		t.Fatal("Allocate không trả về — tràn số làm vòng rải phần dư " +
			"chạy tới ~9e18 lần, và một request treo giữ luôn kết nối " +
			"database của nó")
	}
}

// TestTongTyLeTranThiBaoLoi: tổng tỷ lệ cũng tràn được.
//
// Mười dòng, mỗi dòng 2 tỷ là đủ vượt trần khi cộng dồn — và sau khi tràn
// thì `totalRatio` âm, mọi phép chia sau đó vô nghĩa.
func TestTongTyLeTranThiBaoLoi(t *testing.T) {
	m, err := money.New(1_000_000, "VND")
	if err != nil {
		t.Fatal(err)
	}
	tyLe := []int64{math.MaxInt64 - 10, 100}
	if _, err := m.Allocate(tyLe); !errors.Is(err, money.ErrTranSo) {
		t.Errorf("tổng tỷ lệ tràn mà lỗi là %v, cần ErrTranSo — "+
			"tràn im lặng làm totalRatio ÂM và mọi phép chia sau đó "+
			"cho ra số vô nghĩa", err)
	}
}
