package seller_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func newModule(t *testing.T) (*seller.Module, *database.DB) {
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

	if _, err := db.Pool().Exec(context.Background(), "DELETE FROM seller"); err != nil {
		t.Fatalf("dọn dữ liệu: %v", err)
	}

	m, err := seller.New(seller.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("seller.New: %v", err)
	}
	return m, db
}

func apply(t *testing.T, m *seller.Module, slug string) *seller.SellerView {
	t.Helper()
	v, err := m.ApplyAsSeller(context.Background(), seller.ApplicationRequest{
		Name:             "Cửa hàng " + slug,
		Slug:             slug,
		SellerType:       "BUSINESS",
		Email:            "shop@example.com",
		CommissionRateBP: 1000, // 10%
	})
	if err != nil {
		t.Fatalf("ApplyAsSeller: %v", err)
	}
	return v
}

func TestNopHoSoBatDauOTrangThaiApplied(t *testing.T) {
	m, _ := newModule(t)
	v := apply(t, m, "cua-hang-a")

	if v.Status != "APPLIED" {
		t.Errorf("trạng thái = %q, mong APPLIED", v.Status)
	}
	if v.IsActive {
		t.Error("hồ sơ mới nộp không được coi là đang hoạt động")
	}
	if v.CommissionRateBP != 1000 {
		t.Errorf("hoa hồng = %d, mong 1000", v.CommissionRateBP)
	}
}

// QUY TẮC 1 (mục 10): nhà bán ACTIVE phải có tài khoản ngân hàng đã xác minh.
//
// Không có thì seller bán được hàng nhưng không nhận được tiền — tranh chấp
// không đáng có, và nền tảng giữ tiền hộ mà không biết trả về đâu.
func TestKhongKichHoatDuocKhiChuaCoTaiKhoanNganHang(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()
	v := apply(t, m, "chua-co-bank")
	id := ids.ID(v.ID)

	svc := m.Service()
	if _, err := svc.SubmitForReview(ctx, id); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if err := m.ApproveSeller(ctx, v.ID, "admin"); err != nil {
		t.Fatalf("ApproveSeller: %v", err)
	}

	// Chưa xác minh tài khoản → KHÔNG kích hoạt được.
	if _, err := svc.Activate(ctx, id); err == nil {
		t.Fatal("kích hoạt khi chưa có tài khoản ngân hàng phải bị chặn")
	}

	// Xác minh xong → kích hoạt được.
	if _, err := svc.VerifyBankAccount(ctx, id); err != nil {
		t.Fatalf("VerifyBankAccount: %v", err)
	}
	got, err := svc.Activate(ctx, id)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !got.IsActive() {
		t.Error("sau khi xác minh tài khoản phải kích hoạt được")
	}
}

// Own brand là seller INTERNAL, KHÔNG phải đường đi riêng (mục 3).
//
// Nhờ vậy đơn hàng lẫn own brand và hàng seller đi CHUNG một luồng.
func TestOwnBrandLaSellerNoiBo(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	sel, err := m.Service().EnsureInternalSeller(ctx, "Lumière", seller.InternalSellerSlug)
	if err != nil {
		t.Fatalf("EnsureInternalSeller: %v", err)
	}

	if !sel.IsInternal() {
		t.Error("own brand phải là seller INTERNAL")
	}
	// Own brand hoạt động NGAY, không cần duyệt và không cần tài khoản
	// ngân hàng (nhận tiền qua sổ cái nội bộ).
	if !sel.IsActive() {
		t.Errorf("own brand phải ở trạng thái ACTIVE, đang là %q", sel.Status())
	}
	// Nền tảng KHÔNG thu hoa hồng của chính mình.
	if !sel.CommissionRate().IsZero() {
		t.Errorf("hoa hồng own brand = %v, mong 0", sel.CommissionRate())
	}

	// Idempotent: gọi lại trả về CÙNG seller, không tạo bản thứ hai.
	again, err := m.Service().EnsureInternalSeller(ctx, "Lumière", seller.InternalSellerSlug)
	if err != nil {
		t.Fatalf("EnsureInternalSeller lần 2: %v", err)
	}
	if again.ID() != sel.ID() {
		t.Error("gọi lại tạo ra seller nội bộ thứ hai")
	}
}

// Own brand không được đặt hoa hồng: nền tảng không thu của chính mình.
func TestOwnBrandKhongChiuHoaHong(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	// Truyền hoa hồng khác 0 khi đăng ký INTERNAL — phải bị bỏ qua.
	v, err := m.ApplyAsSeller(ctx, seller.ApplicationRequest{
		Name: "Own brand", Slug: "own-2",
		SellerType: "INTERNAL", CommissionRateBP: 2000,
	})
	if err != nil {
		t.Fatalf("ApplyAsSeller: %v", err)
	}
	if v.CommissionRateBP != 0 {
		t.Errorf("hoa hồng = %d, mong 0 — own brand không chịu hoa hồng", v.CommissionRateBP)
	}
}

// Đình chỉ làm ẩn offer nhưng KHÔNG hủy đơn (mục 5).
//
// Điểm dễ sai: đình chỉ seller không được hủy đơn hàng khách đã trả tiền.
func TestDinhChiLamAnOfferVaPhaiCoLyDo(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()
	v := activeSeller(t, m, "bi-dinh-chi")

	// Đình chỉ không lý do → bị chặn. Seller cần biết vì sao để khắc phục.
	if err := m.SuspendSeller(ctx, v.ID, ""); err == nil {
		t.Error("đình chỉ không nêu lý do phải bị chặn")
	}

	if err := m.SuspendSeller(ctx, v.ID, "Tỷ lệ hủy đơn vượt 3%"); err != nil {
		t.Fatalf("SuspendSeller: %v", err)
	}

	got, err := m.GetSeller(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetSeller: %v", err)
	}
	if got.IsActive {
		t.Error("seller bị đình chỉ không được coi là đang hoạt động")
	}
	if !got.OffersHidden {
		t.Error("seller bị đình chỉ phải làm ẩn offer")
	}

	// Khôi phục được.
	if _, err := m.Service().Reactivate(ctx, ids.ID(v.ID)); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	got, err = m.GetSeller(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetSeller: %v", err)
	}
	if !got.IsActive || got.OffersHidden {
		t.Error("sau khi khôi phục, seller phải hoạt động và offer hiện lại")
	}
}

// Chấm dứt là trạng thái CUỐI — không có đường quay lại.
//
// Đã đối soát lần cuối và chi trả số dư; mở lại sẽ làm hỏng sổ sách.
func TestChamDutLaTrangThaiCuoi(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()
	v := activeSeller(t, m, "cham-dut")
	id := ids.ID(v.ID)

	if _, err := m.Service().Terminate(ctx, id, "Vi phạm chính sách nghiêm trọng"); err != nil {
		t.Fatalf("Terminate: %v", err)
	}

	for ten, fn := range map[string]func() error{
		"khôi phục": func() error { _, err := m.Service().Reactivate(ctx, id); return err },
		"kích hoạt": func() error { _, err := m.Service().Activate(ctx, id); return err },
		"đình chỉ":  func() error { _, err := m.Service().Suspend(ctx, id, "x"); return err },
	} {
		if err := fn(); err == nil {
			t.Errorf("%s sau khi chấm dứt phải bị chặn", ten)
		}
	}
}

// Marketplace gọi IsSellerActive TRƯỚC khi hiển thị offer.
func TestKiemTraSellerDangHoatDong(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	hoatDong := activeSeller(t, m, "dang-hoat-dong")
	chuaDuyet := apply(t, m, "chua-duyet")

	active, err := m.IsSellerActive(ctx, hoatDong.ID)
	if err != nil {
		t.Fatalf("IsSellerActive: %v", err)
	}
	if !active {
		t.Error("seller đã kích hoạt phải trả true")
	}

	active, err = m.IsSellerActive(ctx, chuaDuyet.ID)
	if err != nil {
		t.Fatalf("IsSellerActive: %v", err)
	}
	if active {
		t.Error("seller chưa duyệt phải trả false")
	}
}

func TestLaySellerTheoLo(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	a := apply(t, m, "lo-a")
	b := apply(t, m, "lo-b")
	khongCo := ids.MustNew(ids.PrefixSeller).String()

	got, err := m.GetSellersByIDs(ctx, []string{a.ID, b.ID, khongCo, "id-sai-dinh-dang"})
	if err != nil {
		t.Fatalf("GetSellersByIDs: %v", err)
	}
	// id không tồn tại và id sai định dạng bị bỏ qua, không làm hỏng lời gọi.
	if len(got) != 2 {
		t.Errorf("số kết quả = %d, mong 2", len(got))
	}
}

func TestSlugTrungBiChan(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()
	apply(t, m, "trung-slug")

	_, err := m.ApplyAsSeller(ctx, seller.ApplicationRequest{
		Name: "Khác", Slug: "trung-slug", SellerType: "BUSINESS",
	})
	if err == nil {
		t.Error("slug trùng phải bị chặn")
	}
}

func TestIDSaiDinhDangTraErrInvalidID(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	if _, err := m.GetSeller(ctx, "khong-phai-id"); !errors.Is(err, seller.ErrInvalidID) {
		t.Errorf("lỗi = %v, mong ErrInvalidID", err)
	}
	if _, err := m.IsSellerActive(ctx, "prd_01KZV27T7DZ04AMAPE1A2W60EY"); !errors.Is(err, seller.ErrInvalidID) {
		t.Errorf("lỗi = %v, mong ErrInvalidID", err)
	}
}

func TestChiHoTroPostgres(t *testing.T) {
	if _, err := seller.New(seller.Config{Storage: "memory"}); err == nil {
		t.Error("mong lỗi với kho lưu trữ memory")
	}
	if _, err := seller.New(seller.Config{Storage: "postgres"}); err == nil {
		t.Error("mong lỗi khi thiếu kết nối database")
	}
}

// activeSeller tạo seller đã đi hết vòng đời tới ACTIVE.
func activeSeller(t *testing.T, m *seller.Module, slug string) *seller.SellerView {
	t.Helper()
	ctx := context.Background()
	v := apply(t, m, slug)
	id := ids.ID(v.ID)
	svc := m.Service()

	if _, err := svc.SubmitForReview(ctx, id); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if err := m.ApproveSeller(ctx, v.ID, "admin"); err != nil {
		t.Fatalf("ApproveSeller: %v", err)
	}
	if _, err := svc.VerifyBankAccount(ctx, id); err != nil {
		t.Fatalf("VerifyBankAccount: %v", err)
	}
	if _, err := svc.Activate(ctx, id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	got, err := m.GetSeller(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetSeller: %v", err)
	}
	return got
}
