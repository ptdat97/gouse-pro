// Package webhook ghi nhật ký webhook đến và cưỡng chế idempotency.
//
// Không biết gì về nghiệp vụ: nó chỉ trả lời "sự kiện này đã nhận chưa"
// và giữ lại nguyên văn những gì bên ngoài đã gửi.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Recorder ghi webhook đến vào bảng webhook_event.
type Recorder struct {
	pool *pgxpool.Pool
}

func NewRecorder(pool *pgxpool.Pool) *Recorder {
	return &Recorder{pool: pool}
}

// SuKien là một webhook đã được ghi nhận.
type SuKien struct {
	ID string

	// DaNhanTruocDo đúng khi nhà cung cấp gửi lại một sự kiện cũ.
	//
	// Bên gọi phải trả 200 và KHÔNG xử lý lại — nếu trả lỗi, nhà cung cấp
	// sẽ gửi tiếp mãi.
	DaNhanTruocDo bool

	// DaXuLyXong đúng khi lần nhận trước đã xử lý thành công.
	//
	// Khác DaNhanTruocDo: một sự kiện có thể đã nhận mà chưa xử lý xong,
	// và khi đó gửi lại là cơ hội để thử lại.
	DaXuLyXong bool
}

// Ghi lưu webhook và cho biết đây có phải lần đầu không.
//
// # Vì sao chỉ mục UNIQUE chứ không phải kiểm tra trước khi ghi
//
// Nhà cung cấp gửi trùng là chuyện thường. Hai bản trùng tới CÙNG LÚC thì
// cả hai cùng thấy "chưa có" nếu ta kiểm bằng một câu SELECT riêng — và
// cả hai cùng xử lý. Chỉ ràng buộc ở database mới nằm cùng giao dịch với
// việc ghi.
func (r *Recorder) Ghi(
	ctx context.Context, nhaCungCap, maSuKien, loaiSuKien string, than []byte,
) (SuKien, error) {
	if !json.Valid(than) {
		return SuKien{}, fmt.Errorf("webhook: thân request không phải JSON hợp lệ")
	}

	id := ids.MustNew(ids.PrefixWebhookEvent).String()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO webhook_event (
			id, provider, provider_event_id, event_type, payload, received_at
		) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, nhaCungCap, maSuKien, loaiSuKien, than, time.Now().UTC())
	if err == nil {
		return SuKien{ID: id}, nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "webhook_event_provider_provider_event_id_key" {
		return SuKien{}, fmt.Errorf("webhook: ghi nhật ký: %w", err)
	}

	// Đã nhận trước đó. Đọc lại để bên gọi biết lần trước có xử lý xong không.
	var cu SuKien
	var xong *time.Time
	err = r.pool.QueryRow(ctx, `
		SELECT id, processed_at FROM webhook_event
		 WHERE provider = $1 AND provider_event_id = $2`,
		nhaCungCap, maSuKien).Scan(&cu.ID, &xong)
	if err != nil {
		return SuKien{}, fmt.Errorf("webhook: đọc lại sự kiện cũ: %w", err)
	}
	cu.DaNhanTruocDo = true
	cu.DaXuLyXong = xong != nil
	return cu, nil
}

// DanhDauXong ghi kết quả xử lý.
//
// loi rỗng nghĩa là thành công. Ngược lại giữ lại lý do và ĐỂ NGỎ
// processed_at, để job thử lại nhặt được.
func (r *Recorder) DanhDauXong(ctx context.Context, id string, loi error) error {
	if loi != nil {
		_, err := r.pool.Exec(ctx,
			`UPDATE webhook_event SET last_error = $2 WHERE id = $1`,
			id, loi.Error())
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_event SET processed_at = $2, last_error = '' WHERE id = $1`,
		id, time.Now().UTC())
	return err
}

// DemChuaXuLy đếm webhook đã nhận mà chưa xử lý xong — chỉ báo giám sát.
//
// Con số này tăng dần nghĩa là có loại sự kiện ta xử lý hỏng liên tục, và
// nhà cung cấp thì đã coi như xong vì ta trả 200.
func (r *Recorder) DemChuaXuLy(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM webhook_event WHERE processed_at IS NULL`).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}
