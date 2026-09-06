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

// dungDoiSoat dựng service chỉ với kho intent — đối soát không đụng sổ cái.
func dungDoiSoat(t *testing.T) (*application.Service, *paymentpg.IntentStore) {
	t.Helper()
	db := testdb.Open(t)
	if _, err := db.Pool().Exec(context.Background(),
		"DELETE FROM payment_intent"); err != nil {
		t.Fatalf("dọn dữ liệu: %v", err)
	}
	store := paymentpg.NewIntentStore(db.Pool())
	return application.NewService(application.Deps{Intents: store}), store
}

func intentDaThu(t *testing.T, svc *application.Service, orderID ids.ID, soTien int64) {
	t.Helper()
	ctx := context.Background()

	m, err := money.New(soTien, money.VND)
	if err != nil {
		t.Fatalf("money.New: %v", err)
	}
	if _, err := svc.TaoIntent(ctx, application.TaoIntentInput{
		OrderID: orderID, Amount: m, PhuongThuc: "CARD",
	}); err != nil {
		t.Fatalf("TaoIntent: %v", err)
	}
	if _, err := svc.DoiChieuVaThu(ctx, orderID, m, "psp", "pi_x"); err != nil {
		t.Fatalf("DoiChieuVaThu: %v", err)
	}
}

// TestDoiSoatBatDuocDonDaThuTienMaChuaCapNhat khóa lưới an toàn cho một lỗ
// hổng ĐÃ BIẾT.
//
// Handler webhook, khi thu tiền xong mà `MarkOrderPaid` hỏng, cố ý KHÔNG
// quay ngược intent — tiền về là sự thật đã xảy ra. Nó ghi log rồi đi
// tiếp, và để lại đúng trạng thái này: tiền trong tài khoản, khách vẫn
// thấy đơn chờ thanh toán.
//
// Trước job đối soát, thứ duy nhất bắt được là người đọc log.
func TestDoiSoatBatDuocDonDaThuTienMaChuaCapNhat(t *testing.T) {
	svc, _ := dungDoiSoat(t)
	ctx := context.Background()

	donKet := ids.MustNew(ids.PrefixOrder) // đã thu tiền, đơn KHÔNG cập nhật
	donTot := ids.MustNew(ids.PrefixOrder) // đã thu tiền, đơn đã PAID
	intentDaThu(t, svc, donKet, 420_000)
	intentDaThu(t, svc, donTot, 150_000)

	trangThai := func(_ context.Context, orderID string) (string, error) {
		if orderID == donKet.String() {
			return "PENDING_PAYMENT", nil
		}
		return "PAID", nil
	}

	lech, err := svc.DoiSoatDaThu(ctx, time.Now().Add(-time.Hour), 100, trangThai)
	if err != nil {
		t.Fatalf("DoiSoatDaThu: %v", err)
	}

	if len(lech) != 1 {
		t.Fatalf("tìm được %d bất nhất, cần đúng 1 — %+v", len(lech), lech)
	}
	if lech[0].OrderID != donKet.String() {
		t.Errorf("bất nhất trỏ tới đơn %s, cần %s", lech[0].OrderID, donKet)
	}
	if lech[0].SoTien != 420_000 {
		t.Errorf("số tiền = %d, cần 420000 — báo cáo phải nói RÕ bao nhiêu "+
			"tiền đang treo, không chỉ 'có bất nhất'", lech[0].SoTien)
	}
}

// TestDonTraKhongRaLaBatNhatNANGHON, không phải lý do bỏ qua.
//
// Có tiền thu cho một mã đơn không đọc được là tình huống xấu hơn hẳn
// "đơn chưa cập nhật". Nuốt lỗi ở đây làm nó biến mất khỏi báo cáo.
func TestDonTraKhongRaVanBaoCaoLaBatNhat(t *testing.T) {
	svc, _ := dungDoiSoat(t)
	ctx := context.Background()

	don := ids.MustNew(ids.PrefixOrder)
	intentDaThu(t, svc, don, 99_000)

	lech, err := svc.DoiSoatDaThu(ctx, time.Now().Add(-time.Hour), 100,
		func(context.Context, string) (string, error) {
			return "", errors.New("không tìm thấy đơn")
		})
	if err != nil {
		t.Fatalf("DoiSoatDaThu: %v", err)
	}
	if len(lech) != 1 {
		t.Fatalf("tìm được %d bất nhất, cần 1 — đơn tra không ra bị nuốt "+
			"mất khỏi báo cáo", len(lech))
	}
}

// TestCuaSoThoiGianGioiHanPhamViQuet — job quét lại toàn bộ lịch sử mỗi
// lượt sẽ nặng dần theo tuổi hệ thống cho tới lúc tự thành sự cố.
func TestCuaSoThoiGianGioiHanPhamViQuet(t *testing.T) {
	svc, store := dungDoiSoat(t)
	ctx := context.Background()

	don := ids.MustNew(ids.PrefixOrder)
	intentDaThu(t, svc, don, 100_000)

	// Cửa sổ bắt đầu SAU thời điểm thu tiền: bản ghi nằm ngoài tầm.
	lech, err := svc.DoiSoatDaThu(ctx, time.Now().Add(time.Hour), 100,
		func(context.Context, string) (string, error) {
			return "PENDING_PAYMENT", nil
		})
	if err != nil {
		t.Fatalf("DoiSoatDaThu: %v", err)
	}
	if len(lech) != 0 {
		t.Errorf("cửa sổ không giới hạn được phạm vi: quét ra %d bản ghi "+
			"nằm ngoài khoảng", len(lech))
	}

	_ = store
}

// TestDemChoThuQuaHanChiDemDUNGLoai — chỉ số theo dõi phải đếm đúng thứ nó
// khai là đang đếm.
//
// Intent ĐÃ THU lọt vào con số này sẽ làm nó tăng theo doanh số thay vì
// theo sự cố, và khi đó biểu đồ nói ngược điều nó cần nói.
func TestDemChoThuQuaHanChiDemDungLoai(t *testing.T) {
	svc, store := dungDoiSoat(t)
	ctx := context.Background()

	// Một intent ĐÃ THU — không được đếm.
	intentDaThu(t, svc, ids.MustNew(ids.PrefixOrder), 100_000)

	// Hai intent CÒN CHỜ.
	for i := 0; i < 2; i++ {
		m, _ := money.New(50_000, money.VND)
		if _, err := svc.TaoIntent(ctx, application.TaoIntentInput{
			OrderID: ids.MustNew(ids.PrefixOrder), Amount: m, PhuongThuc: "CARD",
		}); err != nil {
			t.Fatalf("TaoIntent: %v", err)
		}
	}

	// Mốc ở TƯƠNG LAI: mọi bản ghi đều "cũ hơn" nó.
	n, err := store.DemChoThuQuaHan(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DemChoThuQuaHan: %v", err)
	}
	if n != 2 {
		t.Errorf("đếm được %d, cần 2 — chỉ intent CÒN CHỜ THU mới được tính", n)
	}

	// Mốc ở QUÁ KHỨ: chưa bản ghi nào đủ cũ.
	n, err = store.DemChoThuQuaHan(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("DemChoThuQuaHan: %v", err)
	}
	if n != 0 {
		t.Errorf("đếm được %d, cần 0 — mốc thời gian không lọc gì cả", n)
	}
}

// TestIntentThatBaiKhongVaoDoiSoat — chỉ đơn ĐÃ THU mới cần đối soát.
func TestIntentThatBaiKhongVaoDoiSoat(t *testing.T) {
	svc, _ := dungDoiSoat(t)
	ctx := context.Background()

	don := ids.MustNew(ids.PrefixOrder)
	m, _ := money.New(70_000, money.VND)
	if _, err := svc.TaoIntent(ctx, application.TaoIntentInput{
		OrderID: don, Amount: m, PhuongThuc: "CARD",
	}); err != nil {
		t.Fatalf("TaoIntent: %v", err)
	}
	if _, err := svc.GhiThatBai(ctx, don, "the bi tu choi"); err != nil {
		t.Fatalf("GhiThatBai: %v", err)
	}

	lech, err := svc.DoiSoatDaThu(ctx, time.Now().Add(-time.Hour), 100,
		func(context.Context, string) (string, error) {
			return "PENDING_PAYMENT", nil
		})
	if err != nil {
		t.Fatalf("DoiSoatDaThu: %v", err)
	}
	if len(lech) != 0 {
		t.Errorf("intent THẤT BẠI bị báo là bất nhất (%d) — đơn chờ thanh "+
			"toán sau một lần trả tiền hỏng là ĐÚNG, không phải sự cố", len(lech))
	}

	_ = domain.IntentThatBai
}
