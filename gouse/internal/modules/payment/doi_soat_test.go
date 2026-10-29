package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	paymentpg "github.com/fashion-commerce/platform/internal/modules/payment/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// newReconcileService dựng service chỉ với kho intent — đối soát không đụng sổ cái.
func newReconcileService(t *testing.T) (*application.Service, *paymentpg.IntentStore) {
	t.Helper()
	db := testdb.Open(t)
	if _, err := db.Pool().Exec(context.Background(),
		"DELETE FROM payment_intent"); err != nil {
		t.Fatalf("dọn dữ liệu: %v", err)
	}
	store := paymentpg.NewIntentStore(db.Pool())
	return application.NewService(application.Deps{Intents: store}), store
}

func capturedIntent(t *testing.T, svc *application.Service, orderID ids.ID, amount int64) {
	t.Helper()
	ctx := context.Background()

	m, err := money.New(amount, money.VND)
	if err != nil {
		t.Fatalf("money.New: %v", err)
	}
	if _, err := svc.NewIntent(ctx, application.NewIntentInput{
		OrderID: orderID, Amount: m, Method: "CARD",
	}); err != nil {
		t.Fatalf("NewIntent: %v", err)
	}
	if _, err := svc.MatchAndCapture(ctx, orderID, m, "psp", "pi_x"); err != nil {
		t.Fatalf("MatchAndCapture: %v", err)
	}
}

// TestReconcileCatchesCapturedOrdersThatNeverUpdated khóa lưới an toàn cho một lỗ
// hổng ĐÃ BIẾT.
//
// Handler webhook, khi thu tiền xong mà `MarkOrderPaid` hỏng, cố ý KHÔNG
// quay ngược intent — tiền về là sự thật đã xảy ra. Nó ghi log rồi đi
// tiếp, và để lại đúng trạng thái này: tiền trong tài khoản, khách vẫn
// thấy đơn chờ thanh toán.
//
// Trước job đối soát, thứ duy nhất bắt được là người đọc log.
func TestReconcileCatchesCapturedOrdersThatNeverUpdated(t *testing.T) {
	svc, _ := newReconcileService(t)
	ctx := context.Background()

	stuckOrder := ids.MustNew(ids.PrefixOrder) // đã thu tiền, đơn KHÔNG cập nhật
	goodOrder := ids.MustNew(ids.PrefixOrder)  // đã thu tiền, đơn đã PAID
	capturedIntent(t, svc, stuckOrder, 420_000)
	capturedIntent(t, svc, goodOrder, 150_000)

	status := func(_ context.Context, orderID string) (string, error) {
		if orderID == stuckOrder.String() {
			return "PENDING_PAYMENT", nil
		}
		return "PAID", nil
	}

	discrepancies, err := svc.ReconcileCaptured(ctx, time.Now().Add(-time.Hour), 100, status)
	if err != nil {
		t.Fatalf("ReconcileCaptured: %v", err)
	}

	if len(discrepancies) != 1 {
		t.Fatalf("tìm được %d bất nhất, cần đúng 1 — %+v", len(discrepancies), discrepancies)
	}
	if discrepancies[0].OrderID != stuckOrder.String() {
		t.Errorf("bất nhất trỏ tới đơn %s, cần %s", discrepancies[0].OrderID, stuckOrder)
	}
	if discrepancies[0].Amount != 420_000 {
		t.Errorf("số tiền = %d, cần 420000 — báo cáo phải nói RÕ bao nhiêu "+
			"tiền đang treo, không chỉ 'có bất nhất'", discrepancies[0].Amount)
	}
}

// TestDonTraKhongRaLaBatNhatNANGHON, không phải lý do bỏ qua.
//
// Có tiền thu cho một mã đơn không đọc được là tình huống xấu hơn hẳn
// "đơn chưa cập nhật". Nuốt lỗi ở đây làm nó biến mất khỏi báo cáo.
func TestAnUnresolvableOrderIsStillReportedAsADiscrepancy(t *testing.T) {
	svc, _ := newReconcileService(t)
	ctx := context.Background()

	ord := ids.MustNew(ids.PrefixOrder)
	capturedIntent(t, svc, ord, 99_000)

	discrepancies, err := svc.ReconcileCaptured(ctx, time.Now().Add(-time.Hour), 100,
		func(context.Context, string) (string, error) {
			return "", errors.New("không tìm thấy đơn")
		})
	if err != nil {
		t.Fatalf("ReconcileCaptured: %v", err)
	}
	if len(discrepancies) != 1 {
		t.Fatalf("tìm được %d bất nhất, cần 1 — đơn tra không ra bị nuốt "+
			"mất khỏi báo cáo", len(discrepancies))
	}
}

// TestTheTimeWindowBoundsTheScan — job quét lại toàn bộ lịch sử mỗi
// lượt sẽ nặng dần theo tuổi hệ thống cho tới lúc tự thành sự cố.
func TestTheTimeWindowBoundsTheScan(t *testing.T) {
	svc, store := newReconcileService(t)
	ctx := context.Background()

	ord := ids.MustNew(ids.PrefixOrder)
	capturedIntent(t, svc, ord, 100_000)

	// Cửa sổ bắt đầu SAU thời điểm thu tiền: bản ghi nằm ngoài tầm.
	discrepancies, err := svc.ReconcileCaptured(ctx, time.Now().Add(time.Hour), 100,
		func(context.Context, string) (string, error) {
			return "PENDING_PAYMENT", nil
		})
	if err != nil {
		t.Fatalf("ReconcileCaptured: %v", err)
	}
	if len(discrepancies) != 0 {
		t.Errorf("cửa sổ không giới hạn được phạm vi: quét ra %d bản ghi "+
			"nằm ngoài khoảng", len(discrepancies))
	}

	_ = store
}

// TestDemChoThuQuaHanChiDemDUNGLoai — chỉ số theo dõi phải đếm đúng thứ nó
// khai là đang đếm.
//
// Intent ĐÃ THU lọt vào con số này sẽ làm nó tăng theo doanh số thay vì
// theo sự cố, và khi đó biểu đồ nói ngược điều nó cần nói.
func TestTheOverduePendingCountCountsOnlyItsOwnKind(t *testing.T) {
	svc, store := newReconcileService(t)
	ctx := context.Background()

	// Một intent ĐÃ THU — không được đếm.
	capturedIntent(t, svc, ids.MustNew(ids.PrefixOrder), 100_000)

	// Hai intent CÒN CHỜ.
	for i := 0; i < 2; i++ {
		m, _ := money.New(50_000, money.VND)
		if _, err := svc.NewIntent(ctx, application.NewIntentInput{
			OrderID: ids.MustNew(ids.PrefixOrder), Amount: m, Method: "CARD",
		}); err != nil {
			t.Fatalf("NewIntent: %v", err)
		}
	}

	// Mốc ở TƯƠNG LAI: mọi bản ghi đều "cũ hơn" nó.
	n, err := store.CountOverduePending(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CountOverduePending: %v", err)
	}
	if n != 2 {
		t.Errorf("đếm được %d, cần 2 — chỉ intent CÒN CHỜ THU mới được tính", n)
	}

	// Mốc ở QUÁ KHỨ: chưa bản ghi nào đủ cũ.
	n, err = store.CountOverduePending(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountOverduePending: %v", err)
	}
	if n != 0 {
		t.Errorf("đếm được %d, cần 0 — mốc thời gian không lọc gì cả", n)
	}
}

// TestAFailedIntentStaysOutOfReconciliation — chỉ đơn ĐÃ THU mới cần đối soát.
func TestAFailedIntentStaysOutOfReconciliation(t *testing.T) {
	svc, _ := newReconcileService(t)
	ctx := context.Background()

	ord := ids.MustNew(ids.PrefixOrder)
	m, _ := money.New(70_000, money.VND)
	if _, err := svc.NewIntent(ctx, application.NewIntentInput{
		OrderID: ord, Amount: m, Method: "CARD",
	}); err != nil {
		t.Fatalf("NewIntent: %v", err)
	}
	if _, err := svc.RecordFailure(ctx, ord, "the bi tu choi"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	discrepancies, err := svc.ReconcileCaptured(ctx, time.Now().Add(-time.Hour), 100,
		func(context.Context, string) (string, error) {
			return "PENDING_PAYMENT", nil
		})
	if err != nil {
		t.Fatalf("ReconcileCaptured: %v", err)
	}
	if len(discrepancies) != 0 {
		t.Errorf("intent THẤT BẠI bị báo là bất nhất (%d) — đơn chờ thanh "+
			"toán sau một lần trả tiền hỏng là ĐÚNG, không phải sự cố", len(discrepancies))
	}

	_ = domain.IntentFailed
}
