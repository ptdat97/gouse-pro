package payment_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	paymentpg "github.com/fashion-commerce/platform/internal/modules/payment/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// Khoản nhà bán NỢ chỉ được trừ MỘT lần.
//
// # Lỗi mà bộ bài này canh
//
// Hoàn hàng ghi nợ `SELLER_PAYABLE`. Nếu tiền của đơn ấy đã chuyển sang rút
// được, tài khoản đang chờ thành ÂM. Đợt đối soát trừ phần âm ấy ra khỏi số
// thực chi — nhưng tới 23/09/2026 nó chỉ trừ TRÊN GIẤY: không bút toán nào
// được ghi, nên số âm còn nguyên sau đó.
//
// Job tạo đợt chạy MỖI GIỜ. Một khoản nợ 50.000 sẽ bị trừ lại ở đợt sau, và
// đợt sau nữa — nhà bán mất 50.000 mỗi giờ cho cùng một lần hoàn hàng, mãi
// mãi, vì không gì đưa số âm ấy về 0.
//
// # Vì sao dựng sổ cái bằng SQL thô
//
// Đi qua cả luồng đặt đơn → giao → hoàn tất mất vài giây mỗi lần và dựng ra
// một nhà bán MỚI mỗi lần. Bài này cần đúng điều ngược lại: CÙNG một nhà
// bán, hai đợt liên tiếp. Ba bút toán dưới đây là đúng những gì luồng thật
// để lại, không hơn.

func dungDichVuDoiSoat(t *testing.T) (*application.Service, *paymentpg.LedgerStore, *pgxpool.Pool) {
	t.Helper()
	db := testdb.Open(t)
	p := db.Pool()

	// TRUNCATE chứ không DELETE: trigger bất biến chặn DELETE từng dòng.
	if _, err := p.Exec(context.Background(),
		"TRUNCATE ledger_entry, ledger_line, settlement, settlement_line CASCADE"); err != nil {
		t.Fatalf("dọn sổ cái: %v", err)
	}

	led := paymentpg.NewLedgerStore(p)
	return application.NewService(application.Deps{
		Ledger:      led,
		Balances:    paymentpg.NewBalanceStore(p),
		Settlements: paymentpg.NewSettlementStore(p),
	}), led, p
}

func tien(t *testing.T, x int64) money.Money {
	t.Helper()
	m, err := money.New(x, money.VND)
	if err != nil {
		t.Fatalf("money.New(%d): %v", x, err)
	}
	return m
}

// banDuoc ghi một lần bán ĐÃ hết hạn đổi trả: tiền nằm ở RÚT ĐƯỢC.
//
// Hai bút toán, đúng như luồng thật: doanh thu vào đang chờ, rồi hết hạn
// đổi trả thì chuyển sang rút được.
func banDuoc(t *testing.T, svc *application.Service, led *paymentpg.LedgerStore,
	seller ids.ID, soTien int64,
) {
	t.Helper()
	ctx := context.Background()
	orderID := ids.MustNew(ids.PrefixOrder)
	foID := ids.MustNew(ids.PrefixFulfillmentOrder)

	if _, err := svc.RecordOrderRevenue(ctx, application.RecordOrderRevenueInput{
		OrderID: orderID, SellerID: seller,
		// Không hoa hồng: bài này đo cơ chế thu hồi nợ, và một tỷ lệ hoa
		// hồng chen vào chỉ làm mọi con số mong đợi khó đọc hơn.
		GrossAmount: tien(t, soTien), SellerPayable: tien(t, soTien),
		PlatformRevenue: tien(t, 0), PaymentFee: tien(t, 0), COGS: tien(t, 0),
		CreatedBy: "test",
	}); err != nil {
		t.Fatalf("ghi doanh thu: %v", err)
	}

	if err := svc.ChuyenSangRutDuocWith(ctx, led, application.ChuyenSangRutDuocInput{
		FulfillmentID: foID, SellerID: seller,
		Amount: tien(t, soTien), CreatedBy: "test",
	}); err != nil {
		t.Fatalf("chuyển sang rút được: %v", err)
	}
}

// hoanSauKhiDaRutDuoc ghi khoản hoàn hàng rơi vào tài khoản đã rỗng.
//
// Đây là tình huống ĐÚNG mà `deficit` sinh ra để xử lý: khách xin trả ngày
// 6, hàng về ngày 10, khi tiền đã chuyển sang rút được.
func hoanSauKhiDaRutDuoc(t *testing.T, svc *application.Service, seller ids.ID, soTien int64) {
	t.Helper()
	if _, err := svc.RecordRefund(context.Background(), application.RecordRefundInput{
		OrderID: ids.MustNew(ids.PrefixOrder), SellerID: seller,
		Amount: tien(t, soTien), SellerClawback: tien(t, soTien),
		CreatedBy: "test",
	}); err != nil {
		t.Fatalf("ghi hoàn hàng: %v", err)
	}
}

func soDuDangCho(t *testing.T, svc *application.Service, seller ids.ID) int64 {
	t.Helper()
	b, err := svc.GetBalance(context.Background(), domain.Account{
		Type: domain.AccountSellerPayable, OwnerID: seller,
	})
	if err != nil {
		t.Fatalf("đọc số dư đang chờ: %v", err)
	}
	return b.Amount.Amount()
}

func dotCuaNhaBan(t *testing.T, svc *application.Service, seller ids.ID) []*domain.DoiSoat {
	t.Helper()
	ds, err := svc.DanhSachDoiSoat(context.Background(), seller, 50)
	if err != nil {
		t.Fatalf("đọc đợt: %v", err)
	}
	return ds
}

func taoDot(t *testing.T, svc *application.Service) {
	t.Helper()
	den := time.Now().Add(time.Minute)
	if _, err := svc.TaoDoiSoatChoKy(
		context.Background(), den.Add(-7*24*time.Hour), den, 1000); err != nil {
		t.Fatalf("tạo đợt: %v", err)
	}
}

// ĐÂY LÀ LỖI: khoản nợ bị trừ LẠI ở đợt kế tiếp.
func TestKhoanNoChiBiTruMotLan(t *testing.T) {
	svc, led, _ := dungDichVuDoiSoat(t)
	seller := ids.MustNew(ids.PrefixSeller)

	banDuoc(t, svc, led, seller, 100_000)
	hoanSauKhiDaRutDuoc(t, svc, seller, 50_000)

	if got := soDuDangCho(t, svc, seller); got != -50_000 {
		t.Fatalf("dựng sai tình huống: đang chờ phải là −50.000, nhận %d", got)
	}

	taoDot(t, svc)

	ds := dotCuaNhaBan(t, svc, seller)
	if len(ds) != 1 {
		t.Fatalf("mong 1 đợt, nhận %d", len(ds))
	}
	if got := ds[0].Deficit().Amount(); got != 50_000 {
		t.Fatalf("đợt 1 phải trừ 50.000, nhận %d", got)
	}
	if got := ds[0].Net().Amount(); got != 50_000 {
		t.Fatalf("đợt 1 thực nhận phải là 50.000, nhận %d", got)
	}

	// Khoản nợ ĐÃ được thu: số âm phải về 0, nếu không đợt sau trừ lại.
	if got := soDuDangCho(t, svc, seller); got != 0 {
		t.Fatalf("thu hồi xong đang chờ phải về 0, nhận %d — "+
			"đợt sau sẽ trừ lại khoản này", got)
	}

	// Kỳ sau: bán thêm, và KHÔNG được trừ gì nữa.
	banDuoc(t, svc, led, seller, 80_000)
	taoDot(t, svc)

	ds = dotCuaNhaBan(t, svc, seller)
	if len(ds) != 2 {
		t.Fatalf("mong 2 đợt, nhận %d", len(ds))
	}
	moi := ds[0]
	if moi.Gross().Amount() != 80_000 {
		t.Fatalf("đợt 2 tổng phải là 80.000, nhận %d", moi.Gross().Amount())
	}
	if got := moi.Deficit().Amount(); got != 0 {
		t.Fatalf("đợt 2 KHÔNG được trừ gì (nợ đã thu ở đợt 1), nhận %d", got)
	}
	if got := moi.Net().Amount(); got != 80_000 {
		t.Fatalf("đợt 2 thực nhận phải là 80.000, nhận %d", got)
	}
}

// Nợ LỚN HƠN tổng đợt: thực nhận kẹp về 0, phần còn lại sang kỳ sau.
//
// Nền tảng không đòi tiền mặt ngược từ nhà bán, nên "thực nhận −20.000 ₫"
// là một dòng không ai biết phải làm gì với nó.
func TestNoLonHonTongThiThucNhanBangKhong(t *testing.T) {
	svc, led, _ := dungDichVuDoiSoat(t)
	seller := ids.MustNew(ids.PrefixSeller)

	banDuoc(t, svc, led, seller, 100_000)
	hoanSauKhiDaRutDuoc(t, svc, seller, 90_000)
	banDuoc(t, svc, led, seller, 30_000)

	// Đang chờ: +100.000 −100.000 −90.000 +30.000 −30.000 = −90.000
	if got := soDuDangCho(t, svc, seller); got != -90_000 {
		t.Fatalf("dựng sai tình huống: mong −90.000, nhận %d", got)
	}

	taoDot(t, svc)

	ds := dotCuaNhaBan(t, svc, seller)
	if len(ds) != 1 {
		t.Fatalf("mong 1 đợt, nhận %d", len(ds))
	}
	d := ds[0]
	if d.Gross().Amount() != 130_000 {
		t.Fatalf("tổng đợt phải là 130.000, nhận %d", d.Gross().Amount())
	}
	if d.Net().Amount() != 40_000 {
		t.Fatalf("thực nhận phải là 40.000, nhận %d", d.Net().Amount())
	}
	if d.Deficit().Amount() != 90_000 {
		t.Fatalf("trừ phải là 90.000, nhận %d", d.Deficit().Amount())
	}
}

// Nợ vượt tổng: kẹp về 0 và phần chưa thu ĐƯỢC GIỮ LẠI cho kỳ sau.
func TestPhanNoChuaThuHetChuyenSangKySau(t *testing.T) {
	svc, led, _ := dungDichVuDoiSoat(t)
	seller := ids.MustNew(ids.PrefixSeller)

	banDuoc(t, svc, led, seller, 100_000)
	hoanSauKhiDaRutDuoc(t, svc, seller, 90_000)
	banDuoc(t, svc, led, seller, 30_000)
	// Đang chờ −90.000, nhưng đợt đầu chỉ gom được 30.000 nếu ta tạo đợt
	// TRƯỚC khi có khoản 100.000. Ở đây gom cả hai, nên thu hết 90.000.
	// Bài này đo tình huống NGƯỢC LẠI: nợ lớn hơn tổng.
	hoanSauKhiDaRutDuoc(t, svc, seller, 100_000)

	if got := soDuDangCho(t, svc, seller); got != -190_000 {
		t.Fatalf("dựng sai tình huống: mong −190.000, nhận %d", got)
	}

	taoDot(t, svc)

	d := dotCuaNhaBan(t, svc, seller)[0]
	if d.Net().Amount() != 0 {
		t.Fatalf("thực nhận phải KẸP về 0, nhận %d", d.Net().Amount())
	}
	if d.Deficit().Amount() != 130_000 {
		t.Fatalf("chỉ thu được tới mức tổng đợt (130.000), nhận %d",
			d.Deficit().Amount())
	}

	// Phần chưa thu hết vẫn là số âm, để kỳ sau thu tiếp.
	if got := soDuDangCho(t, svc, seller); got != -60_000 {
		t.Fatalf("phần nợ còn lại phải là −60.000, nhận %d", got)
	}
}

// Không nợ thì KHÔNG ghi bút toán thu hồi nào.
//
// Một bút toán 0 đồng là rác trong sổ cái: nó làm mọi bản kê tài khoản dài
// thêm mà không nói gì.
func TestKhongNoThiKhongGhiButToanThuHoi(t *testing.T) {
	svc, led, pool := dungDichVuDoiSoat(t)
	seller := ids.MustNew(ids.PrefixSeller)

	banDuoc(t, svc, led, seller, 100_000)
	taoDot(t, svc)

	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_entry WHERE reference_type = 'SETTLEMENT'`).
		Scan(&n); err != nil {
		t.Fatalf("đếm bút toán: %v", err)
	}
	if n != 0 {
		t.Fatalf("không nợ mà vẫn ghi %d bút toán thu hồi", n)
	}

	d := dotCuaNhaBan(t, svc, seller)[0]
	if d.Net().Amount() != 100_000 {
		t.Fatalf("thực nhận phải là 100.000, nhận %d", d.Net().Amount())
	}
}

// ------------------------------------------- Hàng rào tiền bị bỏ quên

// Bài dưới đây KHÔNG phải bài duy nhất gác bất biến ấy.
//
// `TestPhatLaiEventKhongDemHaiLuot` (internal/app) đã gác nó ở tầng
// EVENT: phát lại `order.placed` không sinh bút toán thứ hai.
//
// Bài này gác ở tầng USE CASE và đo thêm một điều khác: lần gọi thứ hai
// phải trả về CHÍNH bút toán cũ. Bên gọi coi một mã khác là một khoản
// mới, và coi một lỗi là thất bại cần thử lại — hai cách hỏng mà bài ở
// tầng event không nhìn thấy.

// Ghi doanh thu IDEMPOTENT theo đơn.
//
// Outbox giao ít nhất một lần, nên cùng một `order.placed` tới hai lần là
// chuyện thường. Ghi hai lần nghĩa là nhân đôi số phải trả nhà bán.
//
// Ràng buộc UNIQUE ở database chặn được bút toán thứ hai, nhưng bài này
// đo thêm một điều khác: lần gọi thứ hai phải trả về CHÍNH bút toán cũ,
// không phải một lỗi. Bên gọi coi lỗi là thất bại và sẽ thử lại mãi.
func TestGhiDoanhThuIdempotentTheoDon(t *testing.T) {
	svc, _, pool := dungDichVuDoiSoat(t)
	seller := ids.MustNew(ids.PrefixSeller)
	orderID := ids.MustNew(ids.PrefixOrder)

	ghi := func() *domain.LedgerEntry {
		t.Helper()
		e, err := svc.RecordOrderRevenue(context.Background(),
			application.RecordOrderRevenueInput{
				OrderID: orderID, SellerID: seller,
				GrossAmount: tien(t, 100_000), SellerPayable: tien(t, 90_000),
				PlatformRevenue: tien(t, 10_000), PaymentFee: tien(t, 0),
				COGS: tien(t, 0), CreatedBy: "test",
			})
		if err != nil {
			t.Fatalf("ghi doanh thu: %v", err)
		}
		return e
	}

	dau := ghi()
	lai := ghi()

	if dau.ID() != lai.ID() {
		t.Errorf("gọi lại phải trả CHÍNH bút toán cũ: %s ≠ %s — bên gọi "+
			"coi khác biệt này là một khoản mới", dau.ID(), lai.ID())
	}

	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_entry WHERE reference_id = $1`,
		orderID.String()).Scan(&n); err != nil {
		t.Fatalf("đếm bút toán: %v", err)
	}
	if n != 1 {
		t.Fatalf("một đơn ra %d bút toán doanh thu — số phải trả nhà bán "+
			"bị nhân lên", n)
	}

	if got := soDuDangCho(t, svc, seller); got != 90_000 {
		t.Fatalf("số dư đang chờ phải là 90.000, nhận %d", got)
	}
}
