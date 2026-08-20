package e2e_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// benNhanHayHong là bên nhận GHI DỮ LIỆU rồi THẤT BẠI ở n lần đầu.
//
// Ghi trước rồi mới lỗi là điểm mấu chốt: nếu dispatcher không cuộn ngược,
// phần ghi đó sẽ còn lại. Một bên nhận chỉ trả lỗi mà không ghi gì thì
// không kiểm chứng được điều đó.
type benNhanHayHong struct {
	mu      sync.Mutex
	solan   int
	hongDen int
	daGhi   []string
	pool    interface {
		Exec(context.Context, string, ...any)
	}
}

func (h *benNhanHayHong) Name() string { return "e2e.ben_nhan_hay_hong" }

func (h *benNhanHayHong) EventTypes() []string {
	return []string{eventbus.TypeCheckoutCompleted}
}

func (h *benNhanHayHong) Handle(ctx context.Context, e eventbus.Event) error {
	h.mu.Lock()
	h.solan++
	lan := h.solan
	h.mu.Unlock()

	tx, err := eventbus.MustTxFrom(ctx)
	if err != nil {
		return err
	}

	// GHI vào database bằng giao dịch của dispatcher.
	if _, err := tx.Exec(ctx,
		`INSERT INTO e2e_dau_vet (id, ghi_chu) VALUES ($1, $2)`,
		e.ID.String(), "lần "+string(rune('0'+lan)),
	); err != nil {
		return err
	}

	if lan <= h.hongDen {
		return errors.New("bên nhận hỏng có chủ ý")
	}
	return nil
}

// TestBenNhanHongThiCuonNguocPhanGhiCuaChinhNo — kịch bản "giao dịch cuộn
// ngược", ở đúng chỗ nó quan trọng nhất.
//
// # Bất biến
//
// Dispatcher chạy MỖI bên nhận trong một savepoint riêng, và savepoint đó
// bao cả phần ghi của bên nhận LẪN việc đánh dấu event đã xử lý. Hệ quả
// phải đúng cả hai chiều:
//
//	bên nhận lỗi  → phần ghi của nó BIẾN MẤT, event KHÔNG bị đánh dấu
//	               → lần sau thử lại
//	bên nhận khác → KHÔNG bị kéo theo, việc của họ vẫn còn
//
// # Vì sao không được ghép chung một giao dịch cho mọi bên nhận
//
// Ghép chung thì một bên nhận phụ (ví dụ gửi email) hỏng sẽ cuộn ngược cả
// việc chuyển tồn kho Reserved → Committed. Khi đó tiến trình dọn có thể
// nhả hàng của một đơn ĐÃ THANH TOÁN và bán nó cho người khác.
//
// # Vì sao không được để mỗi bên nhận tự mở giao dịch riêng
//
// Tách rời thì phần ghi có thể thành công trong khi đánh dấu thất bại —
// lần thử lại chạy LẦN THỨ HAI. Với tồn kho, lần thứ hai là hàng không có
// thật.
func TestBenNhanHongThiCuonNguocPhanGhiCuaChinhNo(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	if _, err := w.db.Pool().Exec(ctx, `
		CREATE TABLE IF NOT EXISTS e2e_dau_vet (
			id      TEXT PRIMARY KEY,
			ghi_chu TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("tạo bảng dấu vết: %v", err)
	}
	if _, err := w.db.Pool().Exec(ctx, `TRUNCATE e2e_dau_vet`); err != nil {
		t.Fatalf("dọn bảng dấu vết: %v", err)
	}

	// Hỏng ĐÚNG một lần rồi thành công.
	hong := &benNhanHayHong{hongDen: 1}
	w.bus.Subscribe(hong)

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 5)},
	})

	c, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}
	if _, err := w.checkout.CompleteCheckout(
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String()); err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}

	// Vòng phát đầu tiên: bên nhận hỏng.
	w.drain()

	// 1. Phần ghi của bên nhận hỏng phải BIẾN MẤT.
	if n := demDauVet(t, w); n != 0 {
		t.Errorf("còn %d dòng của bên nhận đã lỗi, cần 0 — không cuộn ngược", n)
	}

	// 2. Bên nhận KHÁC không bị kéo theo: tồn kho vẫn chuyển sang cam kết.
	avail, commit := w.stock(skuID, shop)
	if avail != 15 || commit != 5 {
		t.Errorf("tồn kho %d/%d, cần 15/5 — bên nhận hỏng đã kéo bên khác theo",
			avail, commit)
	}

	// Vòng phát thứ hai: thử lại và thành công.
	w.drain()

	if n := demDauVet(t, w); n != 1 {
		t.Errorf("sau khi thử lại còn %d dòng, cần 1 — event không được thử lại", n)
	}

	// 3. Bên nhận ĐÃ THÀNH CÔNG ở vòng trước không chạy lại: tồn kho giữ
	// nguyên chứ không chuyển hai lần.
	//
	// GHI CHÚ VỀ MỨC ĐỘ KIỂM CHỨNG: bảo đảm "đúng một lần" có nhiều lớp —
	// bảng `event_processed` ở dispatcher, và trạng thái domain ở từng
	// module (reservation đã chuyển thì không chuyển lại được). Bỏ riêng
	// lớp `event_processed` KHÔNG làm bài này đỏ, nên nó chứng minh KẾT
	// QUẢ đúng chứ không chứng minh lớp nào tạo ra kết quả đó. Lớp
	// dispatcher có test riêng ở internal/platform/eventbus.
	avail2, commit2 := w.stock(skuID, shop)
	if avail2 != 15 || commit2 != 5 {
		t.Errorf("tồn kho %d/%d sau lần thử lại, cần 15/5 — xử lý hai lần",
			avail2, commit2)
	}
}

func demDauVet(t *testing.T, w *world) int {
	t.Helper()
	var n int
	if err := w.db.Pool().QueryRow(
		context.Background(), `SELECT count(*) FROM e2e_dau_vet`,
	).Scan(&n); err != nil {
		t.Fatalf("đếm dấu vết: %v", err)
	}
	return n
}

var _ eventbus.Handler = (*benNhanHayHong)(nil)
