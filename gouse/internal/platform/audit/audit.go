// Package audit ghi và đọc nhật ký thao tác.
//
// Theo ADR-0011: đây là năng lực PLATFORM, không phải module nghiệp vụ. Nó
// bị mọi module gọi và không sở hữu khái niệm nghiệp vụ nào — giống logger
// và eventbus.
//
// # Package này CHỈ có Write và Query
//
// KHÔNG có Update. KHÔNG có Delete. Không phải vì quên, mà vì một bản ghi
// kiểm toán chỉ đáng tin khi không ai — kể cả người có quyền cao nhất —
// sửa được nó sau khi sự việc xảy ra.
//
// Không có hàm để gọi thì không có đường nào lạm dụng. Database còn chặn
// một lớp nữa bằng trigger (migration 000020), phòng những đường không đi
// qua code này: thao tác thủ công, script di trú, hoặc lỗi code tương lai.
//
// # Vì sao resource_type là chuỗi thuần
//
// Quy tắc R3 của archcheck: platform không được import module nghiệp vụ.
// Nếu audit biết "SELLER" là gì, nó không còn trung lập, và mỗi lần một
// module thêm loại tài nguyên là phải sửa platform.
//
// Cái giá: gõ sai "SELER" thay vì "SELLER" chỉ lộ ra khi truy vấn không
// thấy gì. Giảm nhẹ bằng các hằng số khai báo ở cuối file này — chuỗi
// thuần, không import gì.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Loại người thực hiện.
//
// Phân biệt người thật với hệ thống: "đơn bị hủy" do nhân viên bấm nút
// khác hẳn "đơn bị hủy" do job quá hạn chạy.
const (
	ActorUser      = "USER"
	ActorSystem    = "SYSTEM"
	ActorAPIClient = "API_CLIENT"
)

// Loại tài nguyên, khớp enum trong api/paths/admin.yaml#/audit_log.
//
// Chuỗi thuần, không import module nào — xem ghi chú đầu package.
const (
	ResourceLedger    = "LEDGER"
	ResourceInventory = "INVENTORY"
	ResourceSeller    = "SELLER"
	ResourceCreator   = "CREATOR"
	ResourceCustomer  = "CUSTOMER"
	ResourceContent   = "CONTENT"
	ResourceOrder     = "ORDER"
	ResourceConfig    = "CONFIG"
)

// minReasonLen là độ dài tối thiểu của lý do cho thao tác nhạy cảm.
//
// Khớp `minLength: 20` trong api/paths/admin.yaml. Lý do trống hoặc "test"
// làm audit log vô giá trị: đọc lại sau sáu tháng không ai hiểu vì sao
// thao tác đó được thực hiện.
const minReasonLen = 20

// ErrReasonRequired là thiếu lý do cho thao tác nhạy cảm.
var ErrReasonRequired = errors.New("audit: thao tác nhạy cảm bắt buộc có lý do")

// ErrInvalidEntry là bản ghi thiếu trường bắt buộc.
var ErrInvalidEntry = errors.New("audit: bản ghi không hợp lệ")

// junkReasons là các lý do bị từ chối dù đủ độ dài.
//
// Người vội sẽ gõ cho đủ ký tự. Danh sách này bắt các mẫu phổ biến nhất;
// nó KHÔNG chặn được người cố tình, và không cố làm thế — mục tiêu là
// nhắc người đang vội, không phải chống người cố ý.
var junkReasons = []string{"test", "fix", "abc", "xxx", "asdf", "1234", "..."}

// Entry là một bản ghi nhật ký.
type Entry struct {
	// ActorType: USER, SYSTEM, hoặc API_CLIENT.
	ActorType string

	// ActorID là định danh người thực hiện. Rỗng khi ActorType = SYSTEM.
	ActorID string

	// Action dạng "danh_từ.động_từ": ledger.adjust, seller.suspend.
	Action string

	// ResourceType, ResourceID: tài nguyên bị tác động.
	ResourceType string
	ResourceID   string

	// Reason là lý do. BẮT BUỘC với thao tác nhạy cảm — xem WriteSensitive.
	Reason string

	// RequestID nối bản ghi này với chuỗi truy vết của request.
	RequestID string

	// Metadata là chi tiết bổ sung tùy thao tác.
	//
	// KHÔNG đặt dữ liệu cá nhân vào đây: audit log giữ nhiều năm, và một
	// số điện thoại lọt vào metadata sẽ sống lâu hơn mọi chính sách xóa dữ
	// liệu của chúng ta.
	Metadata map[string]any
}

// Record là một bản ghi đọc ra từ nhật ký.
type Record struct {
	ID           string
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Reason       string
	RequestID    string
	Metadata     map[string]any
	OccurredAt   time.Time
}

// Filter là điều kiện lọc khi đọc nhật ký.
type Filter struct {
	ResourceType string
	ResourceID   string
	Action       string
	ActorID      string

	// From, To giới hạn khoảng thời gian. Giá trị zero nghĩa là không giới hạn.
	From time.Time
	To   time.Time

	// Limit là số bản ghi tối đa. 0 dùng mặc định 20, trần là 100.
	Limit int

	// Cursor là ID của bản ghi cuối trang trước.
	Cursor string
}

// Tx là giao dịch database mà bên gọi đang mở.
//
// Kiểu này là lý do package hoạt động đúng: vết kiểm toán được ghi bằng
// CHÍNH giao dịch của thao tác nghiệp vụ, nên hai thứ cùng thành công hoặc
// cùng thất bại.
//
// Đây cũng là lý do ADR-0011 loại phương án ghi audit qua eventbus: outbox
// chỉ đảm bảo event "cuối cùng" được phát, mà "cuối cùng" là không đủ với
// kiểm toán — event hỏng vào dead letter sau 5 lần là vết kiểm toán KHÔNG
// BAO GIỜ tới.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Recorder ghi và đọc nhật ký thao tác.
type Recorder struct {
	pool *pgxpool.Pool
}

// NewRecorder tạo bộ ghi nhật ký.
func NewRecorder(pool *pgxpool.Pool) *Recorder {
	return &Recorder{pool: pool}
}

// WriteTx ghi một bản ghi BẰNG giao dịch của bên gọi.
//
// Đây là hàm nên dùng cho mọi thao tác thay đổi dữ liệu:
//
//	tx, _ := pool.Begin(ctx)
//	defer tx.Rollback(ctx)
//
//	// ... ghi thay đổi nghiệp vụ bằng tx ...
//	audit.WriteTx(ctx, tx, entry)   // ← CÙNG tx
//
//	tx.Commit(ctx)
//
// Ghi ngoài giao dịch sẽ tạo ra vết kiểm toán cho việc chưa từng xảy ra
// (giao dịch nghiệp vụ rollback nhưng audit đã ghi), hoặc thao tác không
// có vết (audit ghi hỏng sau khi nghiệp vụ đã commit).
func (r *Recorder) WriteTx(ctx context.Context, tx Tx, e Entry) error {
	return writeEntry(ctx, tx, e)
}

// Write ghi một bản ghi KHÔNG kèm giao dịch nghiệp vụ.
//
// Chỉ dùng cho thao tác CHỈ ĐỌC cần ghi vết — điển hình là xem hồ sơ khách
// hàng (admin-api.md mục 6), nơi không có thay đổi dữ liệu nào để gắn vào.
//
// Với thao tác GHI, dùng WriteTx.
func (r *Recorder) Write(ctx context.Context, e Entry) error {
	return writeEntry(ctx, r.pool, e)
}

// WriteSensitive ghi bản ghi cho thao tác nhạy cảm, BẮT BUỘC có lý do.
//
// Bảy thao tác cần lý do theo admin-api.md mục 2: điều chỉnh sổ cái, điều
// chỉnh tồn kho, đình chỉ seller/creator, gỡ nội dung, hủy đơn, hoàn tiền
// ngoài quy trình.
//
// Kiểm tra ở đây là chốt chặn cuối cùng phía server. Frontend cũng kiểm
// tra, nhưng đó chỉ là trải nghiệm — người dùng gọi API trực tiếp được.
func (r *Recorder) WriteSensitive(ctx context.Context, tx Tx, e Entry) error {
	if err := ValidateReason(e.Reason); err != nil {
		return err
	}
	return writeEntry(ctx, tx, e)
}

// ValidateReason kiểm tra lý do có dùng được không.
//
// Trả ErrReasonRequired khi lý do trống, quá ngắn, hoặc là giá trị rác.
func ValidateReason(reason string) error {
	trimmed := strings.TrimSpace(reason)

	if len(trimmed) < minReasonLen {
		return fmt.Errorf("%w: cần tối thiểu %d ký tự, nhận %d",
			ErrReasonRequired, minReasonLen, len(trimmed))
	}

	lower := strings.ToLower(trimmed)
	for _, junk := range junkReasons {
		// Chặn lý do CHỈ gồm một từ rác lặp lại cho đủ độ dài.
		if strings.ReplaceAll(lower, junk, "") == "" {
			return fmt.Errorf("%w: %q không phải lý do có ý nghĩa",
				ErrReasonRequired, trimmed)
		}
	}

	return nil
}

// writeEntry ghi bản ghi qua một bộ thực thi bất kỳ (pool hoặc tx).
func writeEntry(ctx context.Context, ex Tx, e Entry) error {
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("%w: thiếu action", ErrInvalidEntry)
	}
	if strings.TrimSpace(e.ResourceType) == "" {
		return fmt.Errorf("%w: thiếu resource_type", ErrInvalidEntry)
	}

	actorType := e.ActorType
	if actorType == "" {
		// Mặc định SYSTEM chứ không phải USER: quy cho hệ thống một việc
		// người làm thì mất dấu người đó, còn quy cho người một việc hệ
		// thống làm là buộc tội nhầm.
		actorType = ActorSystem
	}

	id, err := ids.New(ids.PrefixAuditLog)
	if err != nil {
		return fmt.Errorf("audit: sinh định danh: %w", err)
	}

	metadata := []byte("{}")
	if len(e.Metadata) > 0 {
		metadata, err = json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("audit: mã hóa metadata: %w", err)
		}
	}

	const q = `
		INSERT INTO audit_log (
			id, actor_type, actor_id, action,
			resource_type, resource_id, reason, request_id, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	if _, err := ex.Exec(ctx, q,
		id.String(), actorType, e.ActorID, e.Action,
		e.ResourceType, e.ResourceID, e.Reason, e.RequestID, metadata,
	); err != nil {
		return fmt.Errorf("audit: ghi nhật ký: %w", err)
	}

	return nil
}

// Giới hạn phân trang, khớp common.yaml#/parameters/Limit.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// Query đọc nhật ký theo bộ lọc, mới nhất trước.
//
// Trả về bản ghi và con trỏ trang tiếp. Con trỏ rỗng nghĩa là hết dữ liệu.
func (r *Recorder) Query(ctx context.Context, f Filter) ([]Record, string, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	// Dựng mệnh đề WHERE theo các bộ lọc có giá trị.
	var (
		conds []string
		args  []any
	)
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if f.ResourceType != "" {
		add("resource_type = $%d", f.ResourceType)
	}
	if f.ResourceID != "" {
		add("resource_id = $%d", f.ResourceID)
	}
	if f.Action != "" {
		add("action = $%d", f.Action)
	}
	if f.ActorID != "" {
		add("actor_id = $%d", f.ActorID)
	}
	if !f.From.IsZero() {
		add("occurred_at >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("occurred_at <= $%d", f.To)
	}
	if f.Cursor != "" {
		// Phân trang theo ID chứ không theo occurred_at: ULID tăng dần theo
		// thời gian, nên thứ tự giống nhau, nhưng ID là DUY NHẤT. Phân
		// trang theo mốc thời gian sẽ bỏ sót hoặc lặp bản ghi khi nhiều
		// bản ghi có cùng dấu thời gian.
		add("id < $%d", f.Cursor)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// Lấy dư MỘT bản ghi để biết còn trang sau hay không, thay vì chạy thêm
	// một truy vấn COUNT trên bảng chỉ có tăng.
	args = append(args, limit+1)

	q := fmt.Sprintf(`
		SELECT id, actor_type, actor_id, action,
		       resource_type, resource_id, reason, request_id,
		       metadata, occurred_at
		FROM audit_log
		%s
		ORDER BY id DESC
		LIMIT $%d`, where, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("audit: đọc nhật ký: %w", err)
	}
	defer rows.Close()

	records := make([]Record, 0, limit)
	for rows.Next() {
		var (
			rec scanRecord
			raw []byte
		)
		if err := rows.Scan(
			&rec.ID, &rec.ActorType, &rec.ActorID, &rec.Action,
			&rec.ResourceType, &rec.ResourceID, &rec.Reason, &rec.RequestID,
			&raw, &rec.OccurredAt,
		); err != nil {
			return nil, "", fmt.Errorf("audit: đọc dòng: %w", err)
		}

		out := Record(rec)
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &out.Metadata); err != nil {
				// Metadata hỏng KHÔNG được làm hỏng cả trang: phần còn lại
				// của bản ghi vẫn là vết kiểm toán hợp lệ.
				out.Metadata = map[string]any{}
			}
		}
		records = append(records, out)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", fmt.Errorf("audit: duyệt kết quả: %w", err)
	}

	next := ""
	if len(records) > limit {
		records = records[:limit]
		next = records[len(records)-1].ID
	}

	return records, next, nil
}

// scanRecord có cùng bố cục với Record để chuyển kiểu trực tiếp khi quét.
type scanRecord Record
