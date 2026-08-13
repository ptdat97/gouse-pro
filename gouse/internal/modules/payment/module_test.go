package payment_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func vnd(n int64) payment.Amount { return payment.Amount{Value: n, Currency: "VND"} }

func newModule(t *testing.T) (*payment.Module, *database.DB) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL để chạy test với PostgreSQL thật")
	}

	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}
	t.Cleanup(db.Close)

	// Sổ cái có trigger chặn DELETE — dùng TRUNCATE (thao tác DDL).
	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE ledger_line CASCADE",
		"TRUNCATE ledger_entry CASCADE",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := payment.New(payment.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("payment.New: %v", err)
	}
	return m, db
}

// VÍ DỤ TỪ ĐẶC TẢ (mục 4.3): đơn marketplace 300.000đ.
//
// Kiểm chứng ĐẦU-CUỐI qua database thật: ghi sổ, đọc lại, tính số dư.
func TestGhiSoDonMarketplace(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	orderID := ids.MustNew(ids.PrefixOrder).String()
	sellerID := ids.MustNew(ids.PrefixSeller).String()
	creatorID := ids.MustNew(ids.PrefixCreator).String()

	entry, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID:         orderID,
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(250500),
		PlatformRevenue: vnd(30000),
		CreatorID:       creatorID,
		CreatorPayable:  vnd(15000),
		PaymentFee:      vnd(4500),
	})
	if err != nil {
		t.Fatalf("RecordOrderRevenue: %v", err)
	}

	if len(entry.Lines) != 5 {
		t.Errorf("số dòng = %d, mong 5", len(entry.Lines))
	}
	if entry.Total.Value != 300000 {
		t.Errorf("tổng = %d, mong 300000", entry.Total.Value)
	}

	// Số dư seller: nền tảng nợ 250.500đ.
	balance, err := m.GetSellerBalance(ctx, sellerID)
	if err != nil {
		t.Fatalf("GetSellerBalance: %v", err)
	}
	if balance.Amount.Value != 250500 {
		t.Errorf("số dư seller = %d, mong 250500", balance.Amount.Value)
	}

	// Tiền mặt nền tảng đang giữ.
	cash, err := m.GetPlatformBalance(ctx, payment.AccountPlatformCash)
	if err != nil {
		t.Fatalf("GetPlatformBalance: %v", err)
	}
	if cash.Amount.Value != 300000 {
		t.Errorf("tiền mặt = %d, mong 300000", cash.Amount.Value)
	}
}

// Đơn own brand ghi doanh thu TOÀN PHẦN + giá vốn riêng.
//
// Đây là phân biệt GMV với doanh thu ngay ở tầng ghi sổ.
func TestGhiSoDonOwnBrand(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()
	orderID := ids.MustNew(ids.PrefixOrder).String()

	if _, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID:         orderID,
		GrossAmount:     vnd(300000),
		PlatformRevenue: vnd(300000), // TOÀN PHẦN
		COGS:            vnd(120000),
	}); err != nil {
		t.Fatalf("RecordOrderRevenue: %v", err)
	}

	entries, err := m.GetEntriesForOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetEntriesForOrder: %v", err)
	}
	// HAI bút toán riêng: doanh thu và giá vốn — hai sự kiện tài chính khác nhau.
	if len(entries) != 2 {
		t.Fatalf("số bút toán = %d, mong 2 (doanh thu + giá vốn)", len(entries))
	}

	rev, err := m.GetPlatformBalance(ctx, payment.AccountPlatformRevenue)
	if err != nil {
		t.Fatalf("GetPlatformBalance: %v", err)
	}
	if rev.Amount.Value != 300000 {
		t.Errorf("doanh thu = %d, mong 300000 (toàn phần)", rev.Amount.Value)
	}

	cogs, err := m.GetPlatformBalance(ctx, payment.AccountCOGS)
	if err != nil {
		t.Fatalf("GetPlatformBalance: %v", err)
	}
	if cogs.Amount.Value != 120000 {
		t.Errorf("giá vốn = %d, mong 120000", cogs.Amount.Value)
	}
}

// IDEMPOTENT: ghi sổ hai lần cùng một đơn KHÔNG nhân đôi số tiền.
//
// Đây là loại lỗi tệ nhất có thể xảy ra ở module này — nền tảng sẽ trả
// seller gấp đôi số tiền thực nhận từ khách.
func TestGhiSoHaiLanKhongNhanDoiSoTien(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	orderID := ids.MustNew(ids.PrefixOrder).String()
	sellerID := ids.MustNew(ids.PrefixSeller).String()

	req := payment.OrderRevenueRequest{
		OrderID:         orderID,
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(270000),
		PlatformRevenue: vnd(30000),
	}

	first, err := m.RecordOrderRevenue(ctx, req)
	if err != nil {
		t.Fatalf("RecordOrderRevenue lần 1: %v", err)
	}

	// Gọi lại BA lần nữa — mô phỏng client thử lại sau timeout.
	for i := 0; i < 3; i++ {
		again, err := m.RecordOrderRevenue(ctx, req)
		if err != nil {
			t.Fatalf("RecordOrderRevenue lần %d: %v", i+2, err)
		}
		// Phải trả về CÙNG bút toán, không tạo bút toán mới.
		if again.ID != first.ID {
			t.Errorf("lần %d tạo bút toán mới %s, mong %s", i+2, again.ID, first.ID)
		}
	}

	// Số dư KHÔNG được nhân lên.
	balance, err := m.GetSellerBalance(ctx, sellerID)
	if err != nil {
		t.Fatalf("GetSellerBalance: %v", err)
	}
	if balance.Amount.Value != 270000 {
		t.Errorf("số dư = %d, mong 270000 — ghi sổ đã bị nhân đôi", balance.Amount.Value)
	}
	if balance.EntryCount != 1 {
		t.Errorf("số bút toán = %d, mong 1", balance.EntryCount)
	}
}

// SỔ CÁI BẤT BIẾN: database TỪ CHỐI mọi UPDATE và DELETE.
//
// ADR-0008: kể cả khi có lỗi code hoặc thao tác thủ công nhầm, database
// vẫn từ chối. Đây là lớp bảo vệ cuối cùng cho tiền của người khác.
func TestSoCaiBatBienOTangDatabase(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	entry, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID:         ids.MustNew(ids.PrefixOrder).String(),
		GrossAmount:     vnd(300000),
		PlatformRevenue: vnd(300000),
	})
	if err != nil {
		t.Fatalf("RecordOrderRevenue: %v", err)
	}

	cases := []struct {
		ten string
		sql string
	}{
		{"UPDATE bút toán", `UPDATE ledger_entry SET description = 'sửa' WHERE id = $1`},
		{"DELETE bút toán", `DELETE FROM ledger_entry WHERE id = $1`},
		{"UPDATE dòng bút toán", `UPDATE ledger_line SET amount = 1 WHERE entry_id = $1`},
		{"DELETE dòng bút toán", `DELETE FROM ledger_line WHERE entry_id = $1`},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			if _, err := db.Pool().Exec(ctx, tc.sql, entry.ID); err == nil {
				t.Errorf("%s THÀNH CÔNG — sổ cái không bất biến!", tc.ten)
			}
		})
	}

	// Nhưng GHI THÊM vẫn hoạt động bình thường.
	if _, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID:         ids.MustNew(ids.PrefixOrder).String(),
		GrossAmount:     vnd(100000),
		PlatformRevenue: vnd(100000),
	}); err != nil {
		t.Errorf("ghi thêm bút toán mới bị chặn: %v", err)
	}
}

// Không chi trả vượt số dư: chi vượt tạo số dư âm, nghĩa là nền tảng đưa
// tiền của mình cho seller mà không có cơ sở.
func TestKhongChiTraVuotSoDu(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	if _, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID:         ids.MustNew(ids.PrefixOrder).String(),
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(270000),
		PlatformRevenue: vnd(30000),
	}); err != nil {
		t.Fatalf("RecordOrderRevenue: %v", err)
	}

	// Chi vượt số dư → bị chặn.
	_, err := m.RecordPayout(ctx, payment.PayoutRequest{
		PayoutID: ids.MustNew(ids.PrefixPayout).String(),
		SellerID: sellerID,
		Amount:   vnd(500000),
	})
	if !errors.Is(err, payment.ErrInsufficientBalance) {
		t.Errorf("lỗi = %v, mong ErrInsufficientBalance", err)
	}

	// Chi trong số dư → được.
	if _, err := m.RecordPayout(ctx, payment.PayoutRequest{
		PayoutID: ids.MustNew(ids.PrefixPayout).String(),
		SellerID: sellerID,
		Amount:   vnd(200000),
	}); err != nil {
		t.Fatalf("RecordPayout: %v", err)
	}

	// Số dư giảm đúng.
	balance, err := m.GetSellerBalance(ctx, sellerID)
	if err != nil {
		t.Fatalf("GetSellerBalance: %v", err)
	}
	if balance.Amount.Value != 70000 {
		t.Errorf("số dư còn = %d, mong 70000", balance.Amount.Value)
	}
}

// Hoàn tiền ĐẢO NGƯỢC chuỗi ghi sổ ban đầu.
func TestHoanTienDaoNguocGhiSo(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	orderID := ids.MustNew(ids.PrefixOrder).String()
	sellerID := ids.MustNew(ids.PrefixSeller).String()

	if _, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID:         orderID,
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(270000),
		PlatformRevenue: vnd(30000),
	}); err != nil {
		t.Fatalf("RecordOrderRevenue: %v", err)
	}

	// Khách hoàn toàn bộ đơn.
	if _, err := m.RecordRefund(ctx, payment.RefundRequest{
		OrderID:          orderID,
		Amount:           vnd(300000),
		SellerID:         sellerID,
		SellerClawback:   vnd(270000),
		PlatformClawback: vnd(30000),
	}); err != nil {
		t.Fatalf("RecordRefund: %v", err)
	}

	// Số dư seller về 0: đã thu hồi hết.
	balance, err := m.GetSellerBalance(ctx, sellerID)
	if err != nil {
		t.Fatalf("GetSellerBalance: %v", err)
	}
	if balance.Amount.Value != 0 {
		t.Errorf("số dư seller = %d, mong 0 sau khi hoàn toàn bộ", balance.Amount.Value)
	}

	// Doanh thu nền tảng cũng về 0.
	rev, err := m.GetPlatformBalance(ctx, payment.AccountPlatformRevenue)
	if err != nil {
		t.Fatalf("GetPlatformBalance: %v", err)
	}
	if rev.Amount.Value != 0 {
		t.Errorf("doanh thu = %d, mong 0", rev.Amount.Value)
	}
}

// TOÀN VẸN SỔ SÁCH: Σ DEBIT = Σ CREDIT toàn hệ thống.
//
// Chỉ số này phải LUÔN bằng 0. Lệch nghĩa là tiền xuất hiện từ hư không.
func TestToanVenSoSach(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	for i := 0; i < 20; i++ {
		if _, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
			OrderID:         ids.MustNew(ids.PrefixOrder).String(),
			GrossAmount:     vnd(300000),
			SellerID:        sellerID,
			SellerPayable:   vnd(270000),
			PlatformRevenue: vnd(30000),
		}); err != nil {
			t.Fatalf("RecordOrderRevenue %d: %v", i, err)
		}
	}

	check, err := m.CheckIntegrity(ctx)
	if err != nil {
		t.Fatalf("CheckIntegrity: %v", err)
	}

	if !check.IsHealthy {
		t.Errorf("sổ sách KHÔNG toàn vẹn: lệch %d, %d bút toán không cân bằng",
			check.Difference, len(check.UnbalancedEntryIDs))
	}
	if check.Difference != 0 {
		t.Errorf("Σ DEBIT − Σ CREDIT = %d, PHẢI bằng 0", check.Difference)
	}
	if check.CheckedEntries != 20 {
		t.Errorf("số bút toán kiểm tra = %d, mong 20", check.CheckedEntries)
	}
}

func TestChiHoTroPostgres(t *testing.T) {
	if _, err := payment.New(payment.Config{Storage: "memory"}); err == nil {
		t.Error("mong lỗi với kho lưu trữ memory")
	}
	if _, err := payment.New(payment.Config{Storage: "postgres"}); err == nil {
		t.Error("mong lỗi khi thiếu kết nối database")
	}
}

func TestIDSaiDinhDangTraErrInvalidID(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	if _, err := m.GetSellerBalance(ctx, "khong-phai-id"); !errors.Is(err, payment.ErrInvalidID) {
		t.Errorf("lỗi = %v, mong ErrInvalidID", err)
	}
	if _, err := m.RecordOrderRevenue(ctx, payment.OrderRevenueRequest{
		OrderID: "sai", GrossAmount: vnd(1000),
	}); !errors.Is(err, payment.ErrInvalidID) {
		t.Errorf("lỗi = %v, mong ErrInvalidID", err)
	}
}
