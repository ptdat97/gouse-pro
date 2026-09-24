package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

// Hàng rào TIỀN của miền trả hàng.
//
// # Vì sao bộ bài này ra đời
//
// Rà ngày 24/09/2026: xóa chốt `ErrDuplicateLine` rồi chạy
// `go test ./internal/modules/returns/... ./internal/app/` — XANH. Gói
// `domain` của returns khi ấy KHÔNG có một file test nào.
//
// Một chốt không ai gác là một chốt sẽ bị "dọn dẹp" đi trong một lần tái
// cấu trúc, và với tiền thì phát hiện muộn không cứu được gì.

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func dongTra(lineID ids.ID, tien int64) domain.Dong {
	return domain.Dong{
		OrderLineID: lineID,
		SKUID:       ids.MustNew(ids.PrefixSKU),
		Quantity:    1,
		TienHoan:    vnd(tien),
		LyDo:        domain.LyDoSizeNho,
	}
}

func taoParams(dong ...domain.Dong) domain.TaoParams {
	return domain.TaoParams{
		OrderID:    ids.MustNew(ids.PrefixOrder),
		SellerID:   ids.MustNew(ids.PrefixSeller),
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Dong:       dong,
		Now:        time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC),
	}
}

// MỘT dòng hàng không được xin trả HAI LẦN trong cùng yêu cầu.
//
// Trùng dòng nghĩa là tiền hoàn của nó được cộng hai lần vào tổng — khách
// nhận gấp đôi cho một món gửi về một lần.
func TestMotDongKhongXinTraHaiLan(t *testing.T) {
	lineID := ids.MustNew(ids.PrefixOrderLine)

	_, err := domain.Tao(taoParams(
		dongTra(lineID, 200_000),
		dongTra(lineID, 200_000),
	))
	if !errors.Is(err, domain.ErrDuplicateLine) {
		t.Fatalf("mong ErrDuplicateLine, nhận %v — tiền hoàn của dòng ấy "+
			"sẽ được cộng hai lần", err)
	}
}

// Hai dòng KHÁC nhau thì hợp lệ, và tổng bằng đúng tổng các dòng.
//
// Không có bài này thì bài trên vẫn xanh khi ai đó chặn nhầm mọi yêu cầu
// nhiều dòng.
func TestTongTienHoanBangTongCacDong(t *testing.T) {
	y, err := domain.Tao(taoParams(
		dongTra(ids.MustNew(ids.PrefixOrderLine), 200_000),
		dongTra(ids.MustNew(ids.PrefixOrderLine), 150_000),
		dongTra(ids.MustNew(ids.PrefixOrderLine), 99_000),
	))
	if err != nil {
		t.Fatalf("ba dòng khác nhau phải hợp lệ: %v", err)
	}

	// Tổng cộng ở MỘT chỗ, trong miền — không nhận từ bên gọi. Hai nguồn
	// cho cùng một con số tiền sớm muộn sẽ lệch nhau.
	if got := y.TienHoan().Amount(); got != 449_000 {
		t.Errorf("tổng tiền hoàn: mong 449.000, nhận %d", got)
	}
}

// Yêu cầu KHÔNG có dòng nào thì từ chối.
//
// Một yêu cầu trả hàng rỗng là một khoản hoàn 0đ mang đầy đủ vết kiểm
// toán của một khoản hoàn thật — rác trong sổ, và một trạng thái không ai
// biết phải làm gì.
func TestYeuCauRongBiTuChoi(t *testing.T) {
	if _, err := domain.Tao(taoParams()); !errors.Is(err, domain.ErrNoLines) {
		t.Fatalf("mong ErrNoLines, nhận %v", err)
	}
}
