package payment_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/platform/audit"
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

	// Dọn cả nhật ký: test điều chỉnh sổ cái kiểm tra vết kiểm toán.
	if _, err := db.Pool().Exec(ctx, "TRUNCATE audit_log"); err != nil {
		t.Fatalf("dọn nhật ký: %v", err)
	}

	m, err := payment.New(payment.Config{
		Storage: "postgres",
		DB:      db,
		Audit:   audit.NewRecorder(db.Pool()),
	})
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

// ---------------------------------------------------------------- Điều chỉnh

// adjustLines dựng hai dòng cân bằng: chuyển tiền từ doanh thu nền tảng
// sang khoản phải trả seller.
func adjustLines(sellerID string, amount int64) []payment.AdjustmentLine {
	return []payment.AdjustmentLine{
		{AccountType: "PLATFORM_REVENUE", Direction: "DEBIT",
			Amount: amount, Currency: "VND"},
		{AccountType: "SELLER_PAYABLE", OwnerID: sellerID, Direction: "CREDIT",
			Amount: amount, Currency: "VND"},
	}
}

// Điều chỉnh sổ cái ghi bút toán VÀ vết kiểm toán trong cùng giao dịch.
//
// Ví dụ từ đặc tả (admin-api.md mục 4): ghi nhầm hoa hồng 12% thay vì 10%.
func TestDieuChinhSoCaiGhiVetKiemToan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	orderID := ids.MustNew(ids.PrefixOrder).String()
	sellerID := ids.MustNew(ids.PrefixSeller).String()
	const reason = "Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10% do lỗi cấu hình ngày 09/08"

	entry, err := m.CreateLedgerAdjustment(ctx, payment.AdjustmentRequest{
		ReferenceType:  "ORDER",
		ReferenceID:    orderID,
		Lines:          adjustLines(sellerID, 5980),
		ActorID:        "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:         reason,
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
	})
	if err != nil {
		t.Fatalf("CreateLedgerAdjustment: %v", err)
	}
	if entry.Type != "ADJUSTMENT" {
		t.Errorf("loại bút toán = %q, mong ADJUSTMENT", entry.Type)
	}
	if entry.Total.Value != 5980 {
		t.Errorf("tổng = %d, mong 5980", entry.Total.Value)
	}

	rec := audit.NewRecorder(db.Pool())
	got, _, err := rec.Query(ctx, audit.Filter{Action: "ledger.adjust"})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mong 1 vết điều chỉnh, nhận %d", len(got))
	}
	if got[0].ActorID == "" {
		t.Error("vết PHẢI có người thực hiện — đây là thao tác tạo tiền trong sổ")
	}
	if got[0].Reason != reason {
		t.Errorf("lý do bị đổi: %q", got[0].Reason)
	}
	if got[0].Metadata["total_amount"] != float64(5980) {
		t.Errorf("vết phải ghi quy mô điều chỉnh: %v", got[0].Metadata)
	}
}

// TestDieuChinhKhongCanBangBiChan — bất biến cốt lõi của sổ cái.
//
// Bút toán không cân nghĩa là tiền xuất hiện từ hư không hoặc biến mất.
func TestDieuChinhKhongCanBangBiChan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	lines := adjustLines(sellerID, 5980)
	lines[1].Amount = 5000 // lệch 980

	_, err := m.CreateLedgerAdjustment(ctx, payment.AdjustmentRequest{
		ReferenceType:  "ORDER",
		ReferenceID:    ids.MustNew(ids.PrefixOrder).String(),
		Lines:          lines,
		ActorID:        "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:         "Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10% do lỗi cấu hình",
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
	})
	if err == nil {
		t.Fatal("bút toán không cân PHẢI bị từ chối")
	}

	// Và KHÔNG để lại vết kiểm toán cho việc chưa xảy ra.
	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{Action: "ledger.adjust"})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("bút toán bị chặn thì KHÔNG được có vết, nhận %d", len(entries))
	}
}

// TestDieuChinhLyDoRacBiChanVaKhongGhiSo là bất biến của P0-6 áp cho sổ cái.
//
// Lý do rác bị từ chối ở bộ ghi vết, và việc từ chối đó phải kéo theo hủy
// CẢ bút toán — nếu không, sổ cái có một khoản tiền không ai giải thích được.
func TestDieuChinhLyDoRacBiChanVaKhongGhiSo(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	_, err := m.CreateLedgerAdjustment(ctx, payment.AdjustmentRequest{
		ReferenceType:  "ORDER",
		ReferenceID:    ids.MustNew(ids.PrefixOrder).String(),
		Lines:          adjustLines(sellerID, 5980),
		ActorID:        "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:         "testtesttesttesttesttest",
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
	})
	if err == nil {
		t.Fatal("lý do rác PHẢI bị từ chối")
	}

	// Sổ cái phải SẠCH: không có bút toán nào ở lại.
	var count int
	if err := db.Pool().QueryRow(ctx,
		"SELECT count(*) FROM ledger_entry WHERE entry_type = 'ADJUSTMENT'").
		Scan(&count); err != nil {
		t.Fatalf("đếm bút toán: %v", err)
	}
	if count != 0 {
		t.Errorf("ghi vết thất bại thì bút toán PHẢI bị hủy theo, còn %d", count)
	}
}

// Gọi lại cùng khóa idempotency KHÔNG ghi bút toán thứ hai.
//
// Nhân đôi một bút toán điều chỉnh là nhân đôi số tiền.
func TestDieuChinhIdempotent(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	req := payment.AdjustmentRequest{
		ReferenceType:  "ORDER",
		ReferenceID:    ids.MustNew(ids.PrefixOrder).String(),
		Lines:          adjustLines(sellerID, 5980),
		ActorID:        "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:         "Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10% do lỗi cấu hình",
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
	}

	first, err := m.CreateLedgerAdjustment(ctx, req)
	if err != nil {
		t.Fatalf("lần 1: %v", err)
	}
	second, err := m.CreateLedgerAdjustment(ctx, req)
	if err != nil {
		t.Fatalf("lần 2 phải thành công (trả kết quả cũ): %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("gọi lại cùng khóa phải trả CÙNG bút toán: %s vs %s",
			first.ID, second.ID)
	}

	var count int
	if err := db.Pool().QueryRow(ctx,
		"SELECT count(*) FROM ledger_entry WHERE entry_type = 'ADJUSTMENT'").
		Scan(&count); err != nil {
		t.Fatalf("đếm bút toán: %v", err)
	}
	if count != 1 {
		t.Errorf("phải có ĐÚNG 1 bút toán, nhận %d", count)
	}
}

// Thiếu đường ghi vết thì từ chối, không âm thầm ghi sổ.
func TestThieuAuditThiTuChoiDieuChinh(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL")
	}
	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}
	t.Cleanup(db.Close)

	// KHÔNG truyền Audit.
	m, err := payment.New(payment.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("payment.New: %v", err)
	}

	_, err = m.CreateLedgerAdjustment(context.Background(), payment.AdjustmentRequest{
		ReferenceType:  "ORDER",
		ReferenceID:    ids.MustNew(ids.PrefixOrder).String(),
		Lines:          adjustLines(ids.MustNew(ids.PrefixSeller).String(), 5980),
		ActorID:        "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:         "Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10% do lỗi cấu hình",
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
	})
	if err == nil {
		t.Error("thiếu AuditRecorder thì điều chỉnh sổ cái phải bị từ chối")
	}
}
