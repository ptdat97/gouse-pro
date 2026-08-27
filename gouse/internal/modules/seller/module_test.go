package seller_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/modules/seller/application"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

func newModule(t *testing.T) (*seller.Module, *database.DB) {
	t.Helper()

	db := testdb.Open(t)

	if _, err := db.Pool().Exec(context.Background(), "DELETE FROM seller"); err != nil {
		t.Fatalf("dọn dữ liệu: %v", err)
	}

	// Dọn cả nhật ký: test đình chỉ kiểm tra vết kiểm toán được ghi.
	if _, err := db.Pool().Exec(context.Background(), "TRUNCATE audit_log"); err != nil {
		t.Fatalf("dọn nhật ký: %v", err)
	}

	m, err := seller.New(seller.Config{
		Storage: "postgres",
		DB:      db,
		Audit:   audit.NewRecorder(db.Pool()),
		MaHoa:   maHoaThu(),
	})
	if err != nil {
		t.Fatalf("seller.New: %v", err)
	}
	return m, db
}

// suspendReq dựng yêu cầu đình chỉ hợp lệ với lý do đủ dài.
func suspendReq(sellerID, reason string) seller.SuspendRequest {
	return seller.SuspendRequest{
		SellerID:   sellerID,
		ActorID:    "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:     reason,
		ReasonCode: "PERFORMANCE_VIOLATION",
	}
}

// approveReq dựng yêu cầu duyệt hợp lệ với hoa hồng 10%.
func approveReq(sellerID string) seller.ApproveRequest {
	return seller.ApproveRequest{
		SellerID:         sellerID,
		ActorID:          "usr_01J9XABC123DEF456GHJKMNPQR",
		CommissionRateBP: 1000,
	}
}

func apply(t *testing.T, m *seller.Module, slug string) *seller.SellerView {
	t.Helper()
	v, err := m.ApplyAsSeller(context.Background(), seller.ApplicationRequest{
		Name:             "Cửa hàng " + slug,
		Slug:             slug,
		SellerType:       "BUSINESS",
		Email:            "shop@example.com",
		CommissionRateBP: 1000, // 10%
		BankAccount: seller.BankAccountInput{
			BankCode:      "VCB",
			AccountNumber: "1903" + ids.MustNew(ids.PrefixSeller).String()[20:],
			AccountHolder: "CHU TAI KHOAN THU",
		},
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
	if _, err := m.ApproveSeller(ctx, approveReq(v.ID)); err != nil {
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
		BankAccount: seller.BankAccountInput{
			BankCode:      "VCB",
			AccountNumber: "1903" + ids.MustNew(ids.PrefixSeller).String()[20:],
			AccountHolder: "CHU TAI KHOAN THU",
		},
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
	if _, err := m.SuspendSeller(ctx, suspendReq(v.ID, "")); err == nil {
		t.Error("đình chỉ không nêu lý do phải bị chặn")
	}

	// Lý do quá ngắn cũng bị chặn: "Tỷ lệ hủy đơn vượt 3%" đọc lại sau sáu
	// tháng không đủ để hiểu chuyện gì đã xảy ra.
	if _, err := m.SuspendSeller(ctx, suspendReq(v.ID, "Tỷ lệ hủy cao")); err == nil {
		t.Error("lý do quá ngắn phải bị chặn")
	}

	res, err := m.SuspendSeller(ctx, suspendReq(v.ID,
		"Tỷ lệ hủy đơn 8% vượt ngưỡng 3% trong 30 ngày liên tiếp"))
	if err != nil {
		t.Fatalf("SuspendSeller: %v", err)
	}
	if res.Note == "" {
		t.Error("phải trả ghi chú tác động: đơn đang xử lý KHÔNG bị hủy")
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

// Duyệt hồ sơ đặt hoa hồng, ghi vết, và nêu rõ bước CHƯA xong.
//
// Điểm dễ hiểu sai nhất: duyệt KHÔNG kích hoạt seller. Người duyệt tưởng
// xong việc rồi không hiểu vì sao gian hàng vẫn im lìm — nên response phải
// nói thẳng ra.
func TestDuyetHoSoDatHoaHongVaGhiVet(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	v := apply(t, m, "cho-duyet")

	if _, err := m.Service().SubmitForReview(ctx, ids.ID(v.ID)); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}

	req := approveReq(v.ID)
	req.CommissionRateBP = 1500 // 15%
	res, err := m.ApproveSeller(ctx, req)
	if err != nil {
		t.Fatalf("ApproveSeller: %v", err)
	}

	if res.Seller.Status != "APPROVED" {
		t.Errorf("trạng thái = %q, mong APPROVED", res.Seller.Status)
	}
	if res.Seller.IsActive {
		t.Error("duyệt KHÔNG được kích hoạt seller — còn thiếu xác minh " +
			"tài khoản ngân hàng")
	}
	if res.Seller.CommissionRateBP != 1500 {
		t.Errorf("hoa hồng = %d, mong 1500", res.Seller.CommissionRateBP)
	}

	// side_effects phải cảnh báo bước còn thiếu.
	var warned bool
	for _, e := range res.SideEffects {
		if strings.Contains(e, "CHƯA kích hoạt") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("side_effects phải nêu rõ seller CHƯA kích hoạt: %v",
			res.SideEffects)
	}

	// Vết kiểm toán có tỷ lệ hoa hồng — cam kết tài chính phải truy được.
	rec := audit.NewRecorder(db.Pool())
	got, _, err := rec.Query(ctx, audit.Filter{
		ResourceID: v.ID,
		Action:     "seller.approve",
	})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mong 1 vết duyệt, nhận %d", len(got))
	}
	if got[0].ActorID == "" {
		t.Error("vết duyệt PHẢI có người duyệt")
	}
	if got[0].Metadata["commission_rate_bp"] != float64(1500) {
		t.Errorf("vết phải ghi tỷ lệ hoa hồng đã cam kết: %v",
			got[0].Metadata)
	}
}

// Hoa hồng ngoài khoảng [0, 10000] bị chặn.
//
// Hoa hồng 150% lọt vào là mọi đơn của seller đó tính sai cho tới khi có
// người để ý — thường là lúc đối soát, đã qua vài kỳ.
func TestHoaHongNgoaiKhoangBiChan(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()
	v := apply(t, m, "hoa-hong-sai")

	if _, err := m.Service().SubmitForReview(ctx, ids.ID(v.ID)); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}

	for _, rate := range []int32{-1, 10001, 15000} {
		req := approveReq(v.ID)
		req.CommissionRateBP = rate
		if _, err := m.ApproveSeller(ctx, req); err == nil {
			t.Errorf("hoa hồng %d phần vạn phải bị chặn", rate)
		}
	}
}

// Duyệt thất bại KHÔNG để lại trạng thái nửa vời.
//
// Cùng bất biến với đình chỉ, nhưng đường khác: ở đây máy trạng thái từ
// chối (hồ sơ chưa nộp duyệt) chứ không phải việc ghi vết.
func TestDuyetHoSoSaiTrangThaiBiChan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	// Hồ sơ vừa nộp, CHƯA chuyển sang PENDING_REVIEW.
	v := apply(t, m, "chua-nop-duyet")

	_, err := m.ApproveSeller(ctx, approveReq(v.ID))
	if err == nil {
		t.Fatal("duyệt hồ sơ chưa nộp review phải bị chặn")
	}

	// Phải là ErrNotAllowed (→ 409 ở tầng HTTP), KHÔNG phải lỗi hệ thống.
	//
	// Trả 500 cho một thao tác sai của người dùng khiến họ tưởng hệ thống
	// hỏng và thử lại mãi. Lỗi này đã xảy ra thật, bắt được lúc chạy server.
	if !errors.Is(err, seller.ErrNotAllowed) {
		t.Errorf("mong ErrNotAllowed (xung đột trạng thái), nhận: %v", err)
	}

	got, err := m.GetSeller(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetSeller: %v", err)
	}
	if got.Status != "APPLIED" {
		t.Errorf("trạng thái phải giữ nguyên APPLIED, nhận %q", got.Status)
	}

	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{ResourceID: v.ID})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("duyệt bị chặn thì KHÔNG được có vết, nhận %d", len(entries))
	}
}

// Đình chỉ phải ghi vết kiểm toán, kèm đủ danh tính và lý do.
//
// Vết thiếu người thực hiện là vết vô dụng: khi seller khiếu nại, câu hỏi
// đầu tiên luôn là "ai quyết định việc này".
func TestDinhChiGhiVetKiemToan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	v := activeSeller(t, m, "co-vet-kiem-toan")

	const reason = "Tỷ lệ hủy đơn 8% vượt ngưỡng 3% trong 30 ngày liên tiếp"
	if _, err := m.SuspendSeller(ctx, suspendReq(v.ID, reason)); err != nil {
		t.Fatalf("SuspendSeller: %v", err)
	}

	// Lọc theo action: hồ sơ này đã có một vết `seller.approve` từ lúc
	// dựng dữ liệu, và đó là hành vi đúng.
	rec := audit.NewRecorder(db.Pool())
	got, _, err := rec.Query(ctx, audit.Filter{
		ResourceID: v.ID,
		Action:     "seller.suspend",
	})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mong đúng 1 vết đình chỉ, nhận %d", len(got))
	}

	e := got[0]
	if e.Action != "seller.suspend" {
		t.Errorf("action = %q, mong seller.suspend", e.Action)
	}
	if e.ActorID == "" {
		t.Error("vết kiểm toán PHẢI có người thực hiện")
	}
	if e.Reason != reason {
		t.Errorf("lý do bị đổi: %q", e.Reason)
	}
	if e.ResourceType != audit.ResourceSeller {
		t.Errorf("resource_type = %q", e.ResourceType)
	}
}

// TestDinhChiThatBaiKhongDeLaiTrangThaiNuaVoi là bất biến của P0-6.
//
// Khi việc ghi vết kiểm toán thất bại (ở đây: lý do rác bị từ chối), việc
// đổi trạng thái seller PHẢI bị hủy theo. Hai kết cục nửa vời đều nguy hiểm:
//
//	Seller bị đình chỉ mà không có vết  → không ai chịu trách nhiệm
//	Có vết mà seller vẫn hoạt động      → bằng chứng cho việc chưa xảy ra
func TestDinhChiThatBaiKhongDeLaiTrangThaiNuaVoi(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	v := activeSeller(t, m, "khong-nua-voi")

	// "test" lặp lại cho đủ 20 ký tự — đúng thứ người vội sẽ gõ.
	_, err := m.SuspendSeller(ctx, suspendReq(v.ID, "testtesttesttesttesttest"))
	if err == nil {
		t.Fatal("lý do rác phải bị từ chối")
	}

	// Trạng thái seller KHÔNG được đổi.
	got, err := m.GetSeller(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetSeller: %v", err)
	}
	if !got.IsActive {
		t.Error("ghi vết thất bại thì việc đình chỉ PHẢI bị hủy theo — " +
			"seller vẫn phải đang hoạt động")
	}
	if got.OffersHidden {
		t.Error("offer không được bị ẩn khi việc đình chỉ đã bị hủy")
	}

	// Và không có vết kiểm toán nào ở lại.
	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{
		ResourceID: v.ID,
		Action:     "seller.suspend",
	})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("giao dịch hủy thì KHÔNG được có vết đình chỉ, nhận %d",
			len(entries))
	}
}

// Thao tác nhạy cảm KHÔNG được chạy khi chưa nối đường ghi vết.
//
// Thà từ chối còn hơn đình chỉ thành công mà im lặng bỏ qua việc ghi vết —
// lỗi nối dây kiểu đó không ai phát hiện cho tới khi cần tra cứu.
func TestThieuAuditThiTuChoiDinhChi(t *testing.T) {
	db := testdb.Open(t)

	// KHÔNG truyền Audit.
	m, err := seller.New(seller.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("seller.New: %v", err)
	}

	_, err = m.SuspendSeller(context.Background(),
		suspendReq("sel_01J9XABC123DEF456GHJKMNPQR",
			"Tỷ lệ hủy đơn 8% vượt ngưỡng 3% trong 30 ngày liên tiếp"))
	if err == nil {
		t.Error("thiếu AuditRecorder thì thao tác nhạy cảm phải bị từ chối")
	}
}

// Ghi vết NGOÀI giao dịch phải bị từ chối.
//
// Lớp phòng vệ này không nằm trên đường đi hiện tại (SaveWithAudit luôn
// cung cấp giao dịch), nhưng nó chặn kiểu lỗi dễ mắc nhất về sau: gọi bộ
// ghi vết từ một use case mới mà quên mở giao dịch. Khi đó vết và thay đổi
// nghiệp vụ tách rời nhau, và nhật ký mất giá trị mà không ai nhận ra.
func TestGhiVetNgoaiGiaoDichBiTuChoi(t *testing.T) {
	db := testdb.Open(t)

	rec := seller.NewAuditRecorder(audit.NewRecorder(db.Pool()))

	// Ngữ cảnh KHÔNG mang giao dịch.
	err := rec.RecordSuspension(context.Background(), application.SuspensionRecord{
		SellerID: ids.ID("sel_01J9XABC123DEF456GHJKMNPQR"),
		ActorID:  "usr_01J9XABC123DEF456GHJKMNPQR",
		Reason:   "Tỷ lệ hủy đơn 8% vượt ngưỡng 3% trong 30 ngày liên tiếp",
	})
	if err == nil {
		t.Fatal("ghi vết ngoài giao dịch PHẢI bị từ chối, không được âm " +
			"thầm ghi rời")
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
		BankAccount: seller.BankAccountInput{
			BankCode:      "VCB",
			AccountNumber: "1903" + ids.MustNew(ids.PrefixSeller).String()[20:],
			AccountHolder: "CHU TAI KHOAN THU",
		},
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
	if _, err := m.ApproveSeller(ctx, approveReq(v.ID)); err != nil {
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

// maHoaThu là bộ mã hóa dùng cho test, khóa CỐ ĐỊNH.
//
// Sinh ngẫu nhiên mỗi lần chạy sẽ khiến dữ liệu mã hóa từ lượt trước không
// đọc lại được, và lỗi ấy trông y hệt lỗi mã hóa thật.
func maHoaThu() *privacy.BoMaHoa {
	bm, err := privacy.NewBoMaHoa("dGVzdC1vbmx5LWtleS0zMi1ieXRlcy1sb25nLXh4eHg=")
	if err != nil {
		panic(err)
	}
	return bm
}
