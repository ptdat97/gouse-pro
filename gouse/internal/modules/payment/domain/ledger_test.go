package domain_test

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

var testNow = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func line(t domain.AccountType, owner ids.ID, dir domain.Direction, amount int64) domain.Line {
	return domain.Line{
		Account:   domain.Account{Type: t, OwnerID: owner},
		Direction: dir,
		Amount:    vnd(amount),
	}
}

func newEntry(t *testing.T, lines ...domain.Line) (*domain.LedgerEntry, error) {
	t.Helper()
	return domain.NewLedgerEntry(domain.NewEntryParams{
		Type:           domain.EntryOrderRevenue,
		ReferenceType:  "ORDER",
		ReferenceID:    ids.MustNew(ids.PrefixOrder),
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
		Lines:          lines,
		Now:            testNow,
	})
}

// BẤT BIẾN CỐT LÕI: Σ DEBIT = Σ CREDIT.
//
// Bút toán không cân bằng nghĩa là tiền xuất hiện từ hư không hoặc biến
// mất. Chỉ báo này phải LUÔN bằng 0.
func TestButToanKhongCanBangBiChan(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)

	cases := []struct {
		ten   string
		lines []domain.Line
	}{
		{
			"credit thiếu so với debit",
			[]domain.Line{
				line(domain.AccountPlatformCash, "", domain.Debit, 300000),
				line(domain.AccountSellerPayable, sellerID, domain.Credit, 250000),
			},
		},
		{
			"credit thừa so với debit",
			[]domain.Line{
				line(domain.AccountPlatformCash, "", domain.Debit, 300000),
				line(domain.AccountSellerPayable, sellerID, domain.Credit, 250000),
				line(domain.AccountPlatformRevenue, "", domain.Credit, 100000),
			},
		},
		{
			"lệch đúng 1 đồng",
			[]domain.Line{
				line(domain.AccountPlatformCash, "", domain.Debit, 300000),
				line(domain.AccountSellerPayable, sellerID, domain.Credit, 299999),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			if _, err := newEntry(t, tc.lines...); !errors.Is(err, domain.ErrUnbalanced) {
				t.Errorf("lỗi = %v, mong ErrUnbalanced", err)
			}
		})
	}
}

// Bút toán kép cần ÍT NHẤT hai dòng: tiền đi từ đâu tới đâu.
func TestButToanPhaiCoItNhatHaiDong(t *testing.T) {
	if _, err := newEntry(t); !errors.Is(err, domain.ErrNoLines) {
		t.Errorf("không dòng nào: lỗi = %v, mong ErrNoLines", err)
	}
	if _, err := newEntry(t,
		line(domain.AccountPlatformCash, "", domain.Debit, 100000),
	); !errors.Is(err, domain.ErrNoLines) {
		t.Errorf("một dòng: lỗi = %v, mong ErrNoLines", err)
	}
}

// Cộng 300.000 VND với 20 USD ra một con số vô nghĩa nhưng trông vẫn
// "cân bằng" — phải chặn tường minh.
func TestKhongChoTronDonViTienTe(t *testing.T) {
	_, err := domain.NewLedgerEntry(domain.NewEntryParams{
		Type:           domain.EntryOrderRevenue,
		ReferenceType:  "ORDER",
		ReferenceID:    ids.MustNew(ids.PrefixOrder),
		IdempotencyKey: "k1",
		Lines: []domain.Line{
			{
				Account:   domain.Account{Type: domain.AccountPlatformCash},
				Direction: domain.Debit,
				Amount:    money.MustNew(100, money.USD),
			},
			{
				Account:   domain.Account{Type: domain.AccountPlatformRevenue},
				Direction: domain.Credit,
				Amount:    vnd(100),
			},
		},
		Now: testNow,
	})
	if !errors.Is(err, domain.ErrMixedCurrency) {
		t.Errorf("lỗi = %v, mong ErrMixedCurrency", err)
	}
}

// Số tiền mỗi dòng LUÔN DƯƠNG — hướng nằm ở Direction.
//
// Một dấu trừ đặt sai chỗ làm bút toán vẫn "cân bằng" nhưng sai hoàn toàn.
func TestSoTienMoiDongPhaiDuong(t *testing.T) {
	for _, amount := range []int64{0, -100000} {
		_, err := newEntry(t,
			line(domain.AccountPlatformCash, "", domain.Debit, amount),
			line(domain.AccountPlatformRevenue, "", domain.Credit, amount),
		)
		if err == nil {
			t.Errorf("số tiền %d phải bị chặn", amount)
		}
	}
}

// Tài khoản phải trả BẮT BUỘC có chủ sở hữu.
//
// SELLER_PAYABLE mà không biết seller nào thì vô nghĩa: không đối soát và
// không chi trả được.
func TestTaiKhoanPhaiTraBatBuocCoChuSoHuu(t *testing.T) {
	for _, at := range []domain.AccountType{
		domain.AccountSellerPayable,
		domain.AccountCreatorPayable,
		domain.AccountCustomerRefundPayable,
		domain.AccountSupplierPayable,
	} {
		if !at.RequiresOwner() {
			t.Errorf("%s phải bắt buộc có chủ sở hữu", at)
		}
		_, err := newEntry(t,
			line(domain.AccountPlatformCash, "", domain.Debit, 100000),
			line(at, "", domain.Credit, 100000), // thiếu chủ sở hữu
		)
		if err == nil {
			t.Errorf("%s không có chủ sở hữu phải bị chặn", at)
		}
	}

	// Tài khoản của nền tảng thì không cần.
	for _, at := range []domain.AccountType{
		domain.AccountPlatformCash, domain.AccountPlatformRevenue,
		domain.AccountCOGS, domain.AccountFeeExpense, domain.AccountInventoryAsset,
	} {
		if at.RequiresOwner() {
			t.Errorf("%s không nên bắt buộc chủ sở hữu", at)
		}
	}
}

// Khóa idempotency là BẮT BUỘC.
//
// Ghi hai lần cùng một sự kiện tài chính sẽ NHÂN ĐÔI số tiền — loại lỗi
// tệ nhất có thể xảy ra ở module này.
func TestKhoaIdempotencyBatBuoc(t *testing.T) {
	_, err := domain.NewLedgerEntry(domain.NewEntryParams{
		Type:          domain.EntryOrderRevenue,
		ReferenceType: "ORDER",
		ReferenceID:   ids.MustNew(ids.PrefixOrder),
		Lines: []domain.Line{
			line(domain.AccountPlatformCash, "", domain.Debit, 100000),
			line(domain.AccountPlatformRevenue, "", domain.Credit, 100000),
		},
		Now: testNow,
	})
	if !errors.Is(err, domain.ErrMissingIdempKey) {
		t.Errorf("lỗi = %v, mong ErrMissingIdempKey", err)
	}
}

// Bút toán phải có tham chiếu nguồn gốc — không thì không truy vết được
// tiền này từ đâu ra.
func TestButToanPhaiCoThamChieuNguonGoc(t *testing.T) {
	_, err := domain.NewLedgerEntry(domain.NewEntryParams{
		Type:           domain.EntryOrderRevenue,
		IdempotencyKey: "k1",
		Lines: []domain.Line{
			line(domain.AccountPlatformCash, "", domain.Debit, 100000),
			line(domain.AccountPlatformRevenue, "", domain.Credit, 100000),
		},
		Now: testNow,
	})
	if !errors.Is(err, domain.ErrMissingReference) {
		t.Errorf("lỗi = %v, mong ErrMissingReference", err)
	}
}

// VÍ DỤ TỪ ĐẶC TẢ (mục 4.3): đơn marketplace 300.000đ.
//
//	DEBIT   PLATFORM_CASH                     300.000
//	CREDIT  SELLER_PAYABLE (seller A)         250.500
//	CREDIT  PLATFORM_REVENUE                   30.000
//	CREDIT  CREATOR_PAYABLE (creator X)        15.000
//	CREDIT  FEE_EXPENSE                         4.500
func TestButToanDonMarketplaceKhopDacTa(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)
	creatorID := ids.MustNew(ids.PrefixCreator)

	e, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
		OrderID:         ids.MustNew(ids.PrefixOrder),
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(250500),
		PlatformRevenue: vnd(30000),
		CreatorID:       creatorID,
		CreatorPayable:  vnd(15000),
		PaymentFee:      vnd(4500),
		IdempotencyKey:  "order-1000-revenue",
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewOrderRevenueEntry: %v", err)
	}

	if len(e.Lines()) != 5 {
		t.Errorf("số dòng = %d, mong 5", len(e.Lines()))
	}
	if e.Total().Amount() != 300000 {
		t.Errorf("tổng = %v, mong 300000", e.Total())
	}
	if !e.IsBalanced() {
		t.Error("bút toán không cân bằng")
	}

	// Số dư: nền tảng giữ 300.000, nợ seller 250.500.
	balances := domain.ComputeBalances([]*domain.LedgerEntry{e})
	cash := balances[domain.Account{Type: domain.AccountPlatformCash}.Key()]
	if cash.Amount.Amount() != 300000 {
		t.Errorf("tiền mặt = %v, mong 300000", cash.Amount)
	}
	sellerAcc := domain.Account{Type: domain.AccountSellerPayable, OwnerID: sellerID}
	if got := balances[sellerAcc.Key()].Amount.Amount(); got != 250500 {
		t.Errorf("phải trả seller = %d, mong 250500", got)
	}
}

// VÍ DỤ TỪ ĐẶC TẢ (mục 4.4): đơn own brand ghi doanh thu TOÀN PHẦN.
//
// Đây là chỗ phân biệt GMV với doanh thu ngay ở tầng ghi sổ: đơn
// marketplace chỉ ghi HOA HỒNG là doanh thu, đơn own brand ghi toàn bộ.
func TestDonOwnBrandGhiDoanhThuToanPhan(t *testing.T) {
	orderID := ids.MustNew(ids.PrefixOrder)

	revenue, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
		OrderID:         orderID,
		GrossAmount:     vnd(300000),
		PlatformRevenue: vnd(300000), // TOÀN PHẦN, không trừ hoa hồng
		IdempotencyKey:  "order-1001-revenue",
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewOrderRevenueEntry: %v", err)
	}

	cogs, err := domain.NewCOGSEntry(orderID, vnd(120000), "order-1001-cogs", "", testNow)
	if err != nil {
		t.Fatalf("NewCOGSEntry: %v", err)
	}

	balances := domain.ComputeBalances([]*domain.LedgerEntry{revenue, cogs})

	// Doanh thu TOÀN PHẦN.
	rev := balances[domain.Account{Type: domain.AccountPlatformRevenue}.Key()]
	if rev.Amount.Amount() != 300000 {
		t.Errorf("doanh thu = %v, mong 300000 (toàn phần)", rev.Amount)
	}

	// KHÔNG có khoản phải trả seller nào.
	for key, b := range balances {
		if b.Account.Type == domain.AccountSellerPayable {
			t.Errorf("đơn own brand không được có SELLER_PAYABLE: %s", key)
		}
	}

	// Giá vốn ghi riêng.
	if got := balances[domain.Account{Type: domain.AccountCOGS}.Key()].Amount.Amount(); got != 120000 {
		t.Errorf("giá vốn = %d, mong 120000", got)
	}
}

// Số dư phải đúng DẤU theo bản chất tài khoản.
//
// Tài sản và chi phí tăng khi ghi NỢ; nợ phải trả và doanh thu tăng khi
// ghi CÓ. Nhầm sẽ ra số âm ở chỗ lẽ ra phải dương.
func TestSoDuDungDauTheoBanChatTaiKhoan(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)

	e, err := newEntry(t,
		line(domain.AccountPlatformCash, "", domain.Debit, 100000),
		line(domain.AccountSellerPayable, sellerID, domain.Credit, 100000),
	)
	if err != nil {
		t.Fatalf("NewLedgerEntry: %v", err)
	}

	balances := domain.ComputeBalances([]*domain.LedgerEntry{e})

	// PLATFORM_CASH là TÀI SẢN: ghi nợ làm tăng → dương.
	cash := balances[domain.Account{Type: domain.AccountPlatformCash}.Key()]
	if cash.Amount.Amount() != 100000 {
		t.Errorf("tiền mặt = %v, mong +100000", cash.Amount)
	}

	// SELLER_PAYABLE là NỢ PHẢI TRẢ: ghi có làm tăng → dương.
	payable := balances[domain.Account{Type: domain.AccountSellerPayable, OwnerID: sellerID}.Key()]
	if payable.Amount.Amount() != 100000 {
		t.Errorf("phải trả seller = %v, mong +100000", payable.Amount)
	}
}

// Chi trả cho seller làm GIẢM khoản phải trả.
func TestChiTraLamGiamKhoanPhaiTra(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)

	revenue, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
		OrderID:         ids.MustNew(ids.PrefixOrder),
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(270000),
		PlatformRevenue: vnd(30000),
		IdempotencyKey:  "rev-1",
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewOrderRevenueEntry: %v", err)
	}

	payout, err := domain.NewPayoutEntry(
		ids.MustNew(ids.PrefixPayout), sellerID, vnd(200000), "payout-1", "", testNow)
	if err != nil {
		t.Fatalf("NewPayoutEntry: %v", err)
	}

	balances := domain.ComputeBalances([]*domain.LedgerEntry{revenue, payout})
	acc := domain.Account{Type: domain.AccountSellerPayable, OwnerID: sellerID}

	// 270.000 phải trả − 200.000 đã trả = 70.000 còn lại.
	if got := balances[acc.Key()].Amount.Amount(); got != 70000 {
		t.Errorf("còn phải trả = %d, mong 70000", got)
	}

	// Tiền mặt: nhận 300.000 − chi 200.000 = 100.000.
	cash := balances[domain.Account{Type: domain.AccountPlatformCash}.Key()]
	if cash.Amount.Amount() != 100000 {
		t.Errorf("tiền mặt = %v, mong 100000", cash.Amount)
	}
}

// SỬA SAI bằng bút toán ĐIỀU CHỈNH, không sửa bút toán cũ (ADR-0008).
//
// Ví dụ: ghi nhầm hoa hồng 30.000đ, đúng phải 25.000đ.
func TestSuaSaiBangButToanDieuChinh(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)
	orderID := ids.MustNew(ids.PrefixOrder)

	// Bút toán GỐC ghi nhầm hoa hồng 30.000 (đúng phải 25.000).
	wrong, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
		OrderID:         orderID,
		GrossAmount:     vnd(300000),
		SellerID:        sellerID,
		SellerPayable:   vnd(270000),
		PlatformRevenue: vnd(30000),
		IdempotencyKey:  "rev-wrong",
		Now:             testNow,
	})
	if err != nil {
		t.Fatalf("NewOrderRevenueEntry: %v", err)
	}

	// Bút toán ĐIỀU CHỈNH: chuyển 5.000 từ doanh thu sang phải trả seller.
	adj, err := domain.NewAdjustmentEntry("ORDER", orderID, []domain.Line{
		line(domain.AccountPlatformRevenue, "", domain.Debit, 5000),
		line(domain.AccountSellerPayable, sellerID, domain.Credit, 5000),
	}, "Điều chỉnh hoa hồng đơn #1000, ghi nhầm tỷ lệ", "adj-1", "admin", testNow)
	if err != nil {
		t.Fatalf("NewAdjustmentEntry: %v", err)
	}

	balances := domain.ComputeBalances([]*domain.LedgerEntry{wrong, adj})

	// Kết quả CUỐI đúng như thể ghi 25.000 ngay từ đầu.
	rev := balances[domain.Account{Type: domain.AccountPlatformRevenue}.Key()]
	if rev.Amount.Amount() != 25000 {
		t.Errorf("doanh thu sau điều chỉnh = %v, mong 25000", rev.Amount)
	}
	payable := balances[domain.Account{Type: domain.AccountSellerPayable, OwnerID: sellerID}.Key()]
	if payable.Amount.Amount() != 275000 {
		t.Errorf("phải trả seller = %v, mong 275000", payable.Amount)
	}

	// Nhưng LỊCH SỬ vẫn còn: hai bút toán, không phải một.
	if rev.EntryCount != 2 {
		t.Errorf("số bút toán chạm vào doanh thu = %d, mong 2 — lịch sử bị mất", rev.EntryCount)
	}
}

// Điều chỉnh KHÔNG có lý do bị chặn: điểm mù trong kiểm toán.
func TestDieuChinhBatBuocCoLyDo(t *testing.T) {
	_, err := domain.NewAdjustmentEntry("ORDER", ids.MustNew(ids.PrefixOrder),
		[]domain.Line{
			line(domain.AccountPlatformRevenue, "", domain.Debit, 5000),
			line(domain.AccountPlatformCash, "", domain.Credit, 5000),
		}, "", "adj-2", "admin", testNow)
	if err == nil {
		t.Error("bút toán điều chỉnh không lý do phải bị chặn")
	}
}

// Kiểm tra toàn vẹn: chỉ báo phải LUÔN bằng 0.
func TestKiemTraToanVenSoSach(t *testing.T) {
	sellerID := ids.MustNew(ids.PrefixSeller)

	var entries []*domain.LedgerEntry
	for i := 0; i < 10; i++ {
		e, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
			OrderID:         ids.MustNew(ids.PrefixOrder),
			GrossAmount:     vnd(300000),
			SellerID:        sellerID,
			SellerPayable:   vnd(270000),
			PlatformRevenue: vnd(30000),
			IdempotencyKey:  ids.MustNew(ids.PrefixRequest).String(),
			Now:             testNow,
		})
		if err != nil {
			t.Fatalf("NewOrderRevenueEntry: %v", err)
		}
		entries = append(entries, e)
	}

	report := domain.CheckIntegrity(entries)
	if !report.IsHealthy() {
		t.Errorf("sổ sách không toàn vẹn: %v", report.UnbalancedEntries)
	}
	if report.TotalEntries != 10 {
		t.Errorf("số bút toán = %d, mong 10", report.TotalEntries)
	}
}

// Bút toán BẤT BIẾN: sửa lát cắt trả về không ảnh hưởng bản gốc.
func TestButToanBatBien(t *testing.T) {
	e, err := newEntry(t,
		line(domain.AccountPlatformCash, "", domain.Debit, 100000),
		line(domain.AccountPlatformRevenue, "", domain.Credit, 100000),
	)
	if err != nil {
		t.Fatalf("NewLedgerEntry: %v", err)
	}

	lines := e.Lines()
	lines[0].Amount = vnd(999999999)

	if e.Lines()[0].Amount.Amount() != 100000 {
		t.Error("sửa được số tiền của bút toán từ bên ngoài")
	}
	if !e.IsBalanced() {
		t.Error("bút toán mất cân bằng sau khi bị can thiệp từ ngoài")
	}
}

// Test theo tính chất: dựng bút toán ngẫu nhiên, mọi bút toán tạo được đều
// PHẢI cân bằng, và tổng số dư toàn hệ thống phải bằng 0.
//
// Tính chất thứ hai là hệ quả toán học của bút toán kép: mỗi đồng ghi nợ ở
// đâu đó đều có một đồng ghi có ở chỗ khác. Nếu không bằng 0, có tiền xuất
// hiện từ hư không.
func TestTinhChatButToanKep(t *testing.T) {
	rng := rand.New(rand.NewSource(20260813))
	sellers := []ids.ID{
		ids.MustNew(ids.PrefixSeller), ids.MustNew(ids.PrefixSeller),
	}

	var entries []*domain.LedgerEntry

	for i := 0; i < 300; i++ {
		gross := int64(10000 + rng.Intn(1000000))
		// Chia gross thành hoa hồng + phần seller, không mất đồng nào.
		commission := gross * int64(5+rng.Intn(20)) / 100
		sellerPart := gross - commission

		e, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
			OrderID:         ids.MustNew(ids.PrefixOrder),
			GrossAmount:     vnd(gross),
			SellerID:        sellers[rng.Intn(len(sellers))],
			SellerPayable:   vnd(sellerPart),
			PlatformRevenue: vnd(commission),
			IdempotencyKey:  ids.MustNew(ids.PrefixRequest).String(),
			Now:             testNow,
		})
		if err != nil {
			t.Fatalf("lần %d: %v", i, err)
		}
		if !e.IsBalanced() {
			t.Fatalf("lần %d: bút toán không cân bằng", i)
		}
		entries = append(entries, e)
	}

	// TÍNH CHẤT: tổng (Σ DEBIT − Σ CREDIT) toàn hệ thống = 0.
	var totalDebit, totalCredit int64
	for _, e := range entries {
		for _, l := range e.Lines() {
			if l.Direction == domain.Debit {
				totalDebit += l.Amount.Amount()
			} else {
				totalCredit += l.Amount.Amount()
			}
		}
	}
	if totalDebit != totalCredit {
		t.Errorf("tổng DEBIT = %d, tổng CREDIT = %d, lệch %d — tiền xuất hiện từ hư không",
			totalDebit, totalCredit, totalDebit-totalCredit)
	}

	if report := domain.CheckIntegrity(entries); !report.IsHealthy() {
		t.Errorf("%d bút toán không cân bằng", len(report.UnbalancedEntries))
	}
}
