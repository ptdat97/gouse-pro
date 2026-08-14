package audit_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func newRecorder(t *testing.T) (*audit.Recorder, *pgxpool.Pool) {
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

	// TRUNCATE chứ không DELETE: trigger bất biến chặn DELETE theo từng
	// dòng, nên dọn dữ liệu test bằng DELETE sẽ thất bại. Đó là hành vi
	// ĐÚNG — xem TestDeleteBlockedByDatabase.
	if _, err := db.Pool().Exec(context.Background(), "TRUNCATE audit_log"); err != nil {
		t.Fatalf("dọn dữ liệu: %v", err)
	}

	return audit.NewRecorder(db.Pool()), db.Pool()
}

func TestWriteAndQuery(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	err := r.Write(ctx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      "usr_01J9XABC123DEF456GHJKMNPQR",
		Action:       "seller.suspend",
		ResourceType: audit.ResourceSeller,
		ResourceID:   "sel_01J9XABC123DEF456GHJKMNPQR",
		Reason:       "Tỷ lệ hủy đơn 8% vượt ngưỡng 3% trong 30 ngày liên tiếp",
		RequestID:    "req_01J9XABC123DEF456GHJKMNPQR",
		Metadata:     map[string]any{"offers_hidden": 142},
	})
	if err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	got, _, err := r.Query(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mong 1 bản ghi, nhận %d", len(got))
	}

	rec := got[0]
	if rec.Action != "seller.suspend" {
		t.Errorf("action: nhận %q", rec.Action)
	}
	if !strings.HasPrefix(rec.ID, "aud_") {
		t.Errorf("ID phải có tiền tố aud_, nhận %q", rec.ID)
	}
	if rec.Metadata["offers_hidden"] != float64(142) {
		t.Errorf("metadata mất dữ liệu: %+v", rec.Metadata)
	}
	if rec.OccurredAt.IsZero() {
		t.Error("occurred_at phải được database điền tự động")
	}
}

// TestUpdateBlockedByDatabase kiểm chứng LỚP BẢO VỆ CUỐI CÙNG.
//
// Tầng Go không có hàm Update, nên test này phải chạy SQL trực tiếp — đúng
// như một thao tác thủ công bằng psql hoặc một script di trú viết vội sẽ
// làm. Nếu database cho sửa, audit log mất hết giá trị.
func TestUpdateBlockedByDatabase(t *testing.T) {
	r, pool := newRecorder(t)
	ctx := context.Background()

	if err := r.Write(ctx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      "usr_1",
		Action:       "ledger.adjust",
		ResourceType: audit.ResourceLedger,
		Reason:       "Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10% do lỗi cấu hình",
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	_, err := pool.Exec(ctx,
		"UPDATE audit_log SET reason = 'lý do đã bị sửa'")
	if err == nil {
		t.Fatal("database PHẢI chặn UPDATE trên audit_log — " +
			"vết kiểm toán sửa được là vết kiểm toán vô giá trị")
	}
	if !strings.Contains(err.Error(), "bất biến") {
		t.Errorf("thông báo lỗi phải nêu rõ lý do, nhận: %v", err)
	}
}

// TestDeleteBlockedByDatabase — cùng lý do với UPDATE.
//
// Xóa vết kiểm toán còn nguy hiểm hơn sửa: sửa còn để lại một bản ghi sai,
// xóa thì không còn dấu nào cho thấy đã có chuyện gì xảy ra.
func TestDeleteBlockedByDatabase(t *testing.T) {
	r, pool := newRecorder(t)
	ctx := context.Background()

	if err := r.Write(ctx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      "usr_1",
		Action:       "customer.view",
		ResourceType: audit.ResourceCustomer,
		Reason:       "Xử lý khiếu nại giao hàng chậm đơn FC-2026-08-001234",
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	if _, err := pool.Exec(ctx, "DELETE FROM audit_log"); err == nil {
		t.Fatal("database PHẢI chặn DELETE trên audit_log")
	}
}

// TestTruncateStillPossible ghi nhận một GIỚI HẠN ĐÃ BIẾT của lớp bảo vệ.
//
// Trigger `FOR EACH ROW` không chạy với TRUNCATE — TRUNCATE không duyệt
// từng dòng. Nghĩa là ai có quyền TRUNCATE vẫn xóa sạch được audit log.
//
// KHÔNG chặn ở đây là có chủ ý: chính test này dùng TRUNCATE để dọn dữ
// liệu, và quyền TRUNCATE ở production được kiểm soát ở tầng phân quyền
// database — tài khoản ứng dụng không có quyền đó. Chặn bằng trigger sẽ
// làm việc dọn dữ liệu khi phát triển trở nên bất khả thi mà không thêm
// được bảo vệ thật nào trước người đã có quyền quản trị database.
//
// Test này tồn tại để giới hạn trên được GHI LẠI thay vì bị phát hiện
// muộn, và để việc thay đổi hành vi này là một quyết định có ý thức.
func TestTruncateStillPossible(t *testing.T) {
	r, pool := newRecorder(t)
	ctx := context.Background()

	if err := r.Write(ctx, audit.Entry{
		ActorType:    audit.ActorSystem,
		Action:       "config.change",
		ResourceType: audit.ResourceConfig,
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	if _, err := pool.Exec(ctx, "TRUNCATE audit_log"); err != nil {
		t.Fatalf("TRUNCATE phải chạy được (giới hạn đã biết): %v", err)
	}

	got, _, err := r.Query(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("sau TRUNCATE phải rỗng, nhận %d bản ghi", len(got))
	}
}

func TestWriteTxRollbackLeavesNoTrace(t *testing.T) {
	r, pool := newRecorder(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("mở giao dịch: %v", err)
	}

	if err := r.WriteTx(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      "usr_1",
		Action:       "order.cancel",
		ResourceType: audit.ResourceOrder,
		Reason:       "Khách yêu cầu hủy vì đặt nhầm size, chưa xuất kho",
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	// Giao dịch nghiệp vụ thất bại → vết kiểm toán KHÔNG được ở lại.
	//
	// Đây là lý do audit phải ghi bằng CHÍNH giao dịch của bên gọi: một
	// vết kiểm toán cho việc chưa từng xảy ra còn tệ hơn không có vết.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	got, _, err := r.Query(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("giao dịch rollback thì KHÔNG được có vết kiểm toán, "+
			"nhận %d bản ghi", len(got))
	}
}

func TestWriteTxCommitPersists(t *testing.T) {
	r, pool := newRecorder(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("mở giao dịch: %v", err)
	}
	if err := r.WriteTx(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      "usr_1",
		Action:       "order.cancel",
		ResourceType: audit.ResourceOrder,
		Reason:       "Khách yêu cầu hủy vì đặt nhầm size, chưa xuất kho",
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, _, err := r.Query(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("commit rồi phải có 1 bản ghi, nhận %d", len(got))
	}
}

func TestQueryFilters(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	entries := []audit.Entry{
		{ActorType: audit.ActorUser, ActorID: "usr_a", Action: "seller.suspend",
			ResourceType: audit.ResourceSeller, ResourceID: "sel_1"},
		{ActorType: audit.ActorUser, ActorID: "usr_b", Action: "ledger.adjust",
			ResourceType: audit.ResourceLedger, ResourceID: "led_1"},
		{ActorType: audit.ActorUser, ActorID: "usr_a", Action: "ledger.adjust",
			ResourceType: audit.ResourceLedger, ResourceID: "led_2"},
	}
	for _, e := range entries {
		if err := r.Write(ctx, e); err != nil {
			t.Fatalf("ghi nhật ký: %v", err)
		}
	}

	cases := map[string]struct {
		filter audit.Filter
		want   int
	}{
		"không lọc":            {audit.Filter{}, 3},
		"theo resource":        {audit.Filter{ResourceType: audit.ResourceLedger}, 2},
		"theo action":          {audit.Filter{Action: "seller.suspend"}, 1},
		"theo người thực hiện": {audit.Filter{ActorID: "usr_a"}, 2},
		"theo resource_id":     {audit.Filter{ResourceID: "led_1"}, 1},
		"kết hợp":              {audit.Filter{ActorID: "usr_a", ResourceType: audit.ResourceLedger}, 1},
		"không khớp":           {audit.Filter{Action: "khong.ton.tai"}, 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, _, err := r.Query(ctx, tc.filter)
			if err != nil {
				t.Fatalf("đọc nhật ký: %v", err)
			}
			if len(got) != tc.want {
				t.Errorf("mong %d bản ghi, nhận %d", tc.want, len(got))
			}
		})
	}
}

func TestQueryNewestFirst(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	for _, action := range []string{"a.one", "b.two", "c.three"} {
		if err := r.Write(ctx, audit.Entry{
			ActorType: audit.ActorSystem, Action: action,
			ResourceType: audit.ResourceConfig,
		}); err != nil {
			t.Fatalf("ghi nhật ký: %v", err)
		}
	}

	got, _, err := r.Query(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("mong 3 bản ghi, nhận %d", len(got))
	}

	// Nhân viên vào trang audit để xem chuyện VỪA xảy ra, không phải chuyện
	// từ đầu năm.
	if got[0].Action != "c.three" {
		t.Errorf("bản ghi mới nhất phải đứng đầu, nhận %q", got[0].Action)
	}
}

func TestQueryPagination(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	const total = 5
	for i := 0; i < total; i++ {
		if err := r.Write(ctx, audit.Entry{
			ActorType: audit.ActorSystem, Action: "config.change",
			ResourceType: audit.ResourceConfig,
		}); err != nil {
			t.Fatalf("ghi nhật ký: %v", err)
		}
	}

	page1, cursor, err := r.Query(ctx, audit.Filter{Limit: 2})
	if err != nil {
		t.Fatalf("đọc trang 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("trang 1 phải có 2 bản ghi, nhận %d", len(page1))
	}
	if cursor == "" {
		t.Fatal("còn dữ liệu thì phải trả con trỏ trang tiếp")
	}

	page2, _, err := r.Query(ctx, audit.Filter{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("đọc trang 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("trang 2 phải có 2 bản ghi, nhận %d", len(page2))
	}

	// Không được lặp bản ghi giữa hai trang.
	for _, a := range page1 {
		for _, b := range page2 {
			if a.ID == b.ID {
				t.Errorf("bản ghi %q xuất hiện ở cả hai trang", a.ID)
			}
		}
	}

	// Trang cuối: hết dữ liệu thì con trỏ phải rỗng.
	_, last, err := r.Query(ctx, audit.Filter{Limit: 100})
	if err != nil {
		t.Fatalf("đọc toàn bộ: %v", err)
	}
	if last != "" {
		t.Errorf("hết dữ liệu thì con trỏ phải rỗng, nhận %q", last)
	}
}

func TestQueryTimeRange(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	if err := r.Write(ctx, audit.Entry{
		ActorType: audit.ActorSystem, Action: "config.change",
		ResourceType: audit.ResourceConfig,
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	now := time.Now().UTC()

	got, _, err := r.Query(ctx, audit.Filter{
		From: now.Add(-time.Hour),
		To:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("bản ghi trong khoảng phải thấy được, nhận %d", len(got))
	}

	got, _, err = r.Query(ctx, audit.Filter{To: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("bản ghi ngoài khoảng KHÔNG được thấy, nhận %d", len(got))
	}
}

func TestWriteRejectsMissingFields(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	cases := map[string]audit.Entry{
		"thiếu action": {
			ActorType: audit.ActorUser, ResourceType: audit.ResourceSeller,
		},
		"thiếu resource_type": {
			ActorType: audit.ActorUser, Action: "seller.suspend",
		},
		"action chỉ có khoảng trắng": {
			ActorType: audit.ActorUser, Action: "   ",
			ResourceType: audit.ResourceSeller,
		},
	}

	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			err := r.Write(ctx, e)
			if !errors.Is(err, audit.ErrInvalidEntry) {
				t.Errorf("phải trả ErrInvalidEntry, nhận: %v", err)
			}
		})
	}
}

// TestActorDefaultsToSystem: thiếu actor_type mặc định là SYSTEM.
//
// Quy cho hệ thống một việc người làm thì mất dấu người đó; quy cho người
// một việc hệ thống làm là buộc tội nhầm. Mặc định an toàn hơn là SYSTEM.
func TestActorDefaultsToSystem(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	if err := r.Write(ctx, audit.Entry{
		Action: "config.change", ResourceType: audit.ResourceConfig,
	}); err != nil {
		t.Fatalf("ghi nhật ký: %v", err)
	}

	got, _, err := r.Query(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if got[0].ActorType != audit.ActorSystem {
		t.Errorf("mặc định phải là SYSTEM, nhận %q", got[0].ActorType)
	}
}

func TestWriteSensitiveRequiresReason(t *testing.T) {
	r, pool := newRecorder(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("mở giao dịch: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = r.WriteSensitive(ctx, tx, audit.Entry{
		ActorType: audit.ActorUser, ActorID: "usr_1",
		Action: "ledger.adjust", ResourceType: audit.ResourceLedger,
		Reason: "", // thiếu lý do
	})
	if !errors.Is(err, audit.ErrReasonRequired) {
		t.Errorf("thao tác nhạy cảm thiếu lý do phải bị từ chối, nhận: %v", err)
	}
}

func TestValidateReason(t *testing.T) {
	cases := map[string]struct {
		reason string
		wantOK bool
	}{
		"lý do thật": {
			"Ghi nhầm tỷ lệ hoa hồng 12% thay vì 10% do lỗi cấu hình ngày 09/08",
			true,
		},
		"đúng 20 ký tự":    {strings.Repeat("a", 20), true},
		"19 ký tự":         {strings.Repeat("a", 19), false},
		"rỗng":             {"", false},
		"chỉ khoảng trắng": {"                         ", false},

		// Người vội sẽ gõ cho đủ ký tự. Những mẫu này bị chặn.
		"test lặp lại": {"testtesttesttesttesttest", false},
		"fix lặp lại":  {"fixfixfixfixfixfixfixfix", false},
		"dấu chấm":     {"........................", false},

		// KHÔNG chặn nhầm lý do thật có chứa từ "test".
		"lý do thật chứa chữ test": {
			"Khách báo lỗi khi test thanh toán, đã hoàn tiền theo yêu cầu",
			true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := audit.ValidateReason(tc.reason)
			if tc.wantOK && err != nil {
				t.Errorf("lý do hợp lệ bị từ chối: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("lý do %q phải bị từ chối", tc.reason)
			}
		})
	}
}

func TestQueryLimitCapped(t *testing.T) {
	r, _ := newRecorder(t)
	ctx := context.Background()

	// Trần 100 chặn client tự đặt limit khổng lồ để kéo cả bảng về —
	// audit_log chỉ có tăng, nên "cả bảng" lớn dần không giới hạn.
	got, _, err := r.Query(ctx, audit.Filter{Limit: 100000})
	if err != nil {
		t.Fatalf("limit quá lớn phải được cắt xuống trần, không phải lỗi: %v", err)
	}
	if len(got) > 100 {
		t.Errorf("limit phải bị cắt xuống 100, nhận %d", len(got))
	}
}
