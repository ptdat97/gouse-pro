package eventbus

import (
	"context"
	"errors"
	"fmt"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dispatcher đọc outbox và đưa event tới các bên nhận.
//
// ĐÂY LÀ THỨ DUY NHẤT PHẢI THAY khi chuyển sang message broker thật. Module
// nghiệp vụ vẫn phát event như cũ, bên nhận vẫn nhận như cũ — chỉ đoạn giữa
// đổi từ "đọc bảng" thành "đọc topic". Đó là lý do outbox được chọn thay vì
// gọi handler trực tiếp lúc phát.
type Dispatcher struct {
	pool     *pgxpool.Pool
	outbox   *Outbox
	log      *slog.Logger
	handlers map[string][]Handler
}

func NewDispatcher(pool *pgxpool.Pool, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		pool:     pool,
		outbox:   NewOutbox(pool),
		log:      log,
		handlers: map[string][]Handler{},
	}
}

// Subscribe đăng ký một bên nhận.
//
// Nhiều bên nhận cho cùng một loại event là bình thường và là mục đích của
// kiến trúc này: `order.placed` có thể có notification, analytics,
// inventory và attribution cùng nghe.
func (d *Dispatcher) Subscribe(h Handler) {
	for _, t := range h.EventTypes() {
		d.handlers[t] = append(d.handlers[t], h)
	}
}

// Outbox trả bộ ghi event, để module nghiệp vụ phát event.
func (d *Dispatcher) Outbox() *Outbox { return d.outbox }

// DispatchBatch xử lý một lô event chưa phát.
//
// Trả về số event đã phát thành công.
//
// MỖI EVENT MỘT GIAO DỊCH RIÊNG. Gộp cả lô vào một giao dịch sẽ khiến một
// event hỏng làm hỏng cả lô — và lần thử lại sẽ chạy lại những event đã
// thành công, tạo thêm việc cho cơ chế idempotency mà không được gì.
func (d *Dispatcher) DispatchBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}

	// Giao dịch NGẮN chỉ để lấy danh sách và khóa hàng.
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("eventbus: mở giao dịch: %w", err)
	}

	batch, err := d.outbox.fetchPending(ctx, tx, limit)
	if err != nil {
		_ = tx.Rollback(ctx)
		return 0, err
	}
	if len(batch) == 0 {
		_ = tx.Rollback(ctx)
		return 0, nil
	}

	var done int
	for _, p := range batch {
		if err := d.dispatchOne(ctx, tx, p); err != nil {
			// Lỗi của MỘT event không dừng cả lô: những event sau nó vẫn
			// phải được thử. Lỗi đã được ghi vào cột last_error.
			d.log.Warn("phát event thất bại",
				"event_id", p.event.ID.String(),
				"event_type", p.event.Type,
				"lần_thử", p.attempts+1,
				"lỗi", err)
			continue
		}
		done++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("eventbus: xác nhận giao dịch: %w", err)
	}
	return done, nil
}

// dispatchOne đưa một event tới mọi bên nhận của nó.
//
// TOÀN BỘ trong giao dịch của bên gọi: xử lý nghiệp vụ của handler, đánh
// dấu đã xử lý, và đánh dấu event đã phát — cùng thành công hoặc cùng
// thất bại.
func (d *Dispatcher) dispatchOne(ctx context.Context, tx pgx.Tx, p pending) error {
	handlers := d.handlers[p.event.Type]

	// Không ai nghe cũng là kết quả HỢP LỆ, không phải lỗi.
	//
	// Đó là điểm cốt lõi của kiến trúc event: bên phát không biết và không
	// cần biết có ai nghe hay không. Đánh dấu đã phát để event không kẹt
	// lại chặn hàng đợi.
	if len(handlers) == 0 {
		return d.outbox.markPublished(ctx, tx, p.rowID)
	}

	for _, h := range handlers {
		if err := d.runHandler(ctx, tx, h, p.event); err != nil {
			// Ghi lỗi bằng CÙNG giao dịch: nếu handler đã ghi gì đó trước
			// khi lỗi, savepoint bên trong runHandler đã hoàn tác nó.
			if markErr := d.outbox.markFailed(ctx, tx, p.rowID, p.attempts, err); markErr != nil {
				return markErr
			}
			return err
		}
	}

	return d.outbox.markPublished(ctx, tx, p.rowID)
}

// runHandler chạy một bên nhận, cưỡng chế idempotency.
//
// ĐÂY LÀ CHỖ LỜI HỨA "bên nhận không xử lý trùng" TRỞ THÀNH SỰ THẬT:
//
//  1. Ghi (event_id, handler) vào event_processed
//  2. Nếu đã tồn tại → BỎ QUA, handler không được gọi
//  3. Nếu chưa → gọi handler bằng CÙNG giao dịch
//
// Bước 1 đứng TRƯỚC bước 3 là chủ ý. Ràng buộc khóa chính của database là
// thứ quyết định ai được chạy, không phải một câu SELECT trước đó — hai
// worker song song đọc cùng lúc sẽ đều thấy "chưa xử lý".
//
// Vì cùng một giao dịch, nếu handler lỗi thì việc đánh dấu cũng bị hoàn
// tác, và lần thử lại sẽ chạy lại handler. Đó là hành vi đúng.
func (d *Dispatcher) runHandler(ctx context.Context, tx pgx.Tx, h Handler, e Event) error {
	// Savepoint để một handler lỗi không làm hỏng giao dịch đang mở —
	// PostgreSQL hủy toàn bộ giao dịch sau một lệnh lỗi nếu không có nó.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("eventbus: mở savepoint: %w", err)
	}

	tag, err := sp.Exec(ctx, `
		INSERT INTO event_processed (event_id, handler)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, e.ID.String(), h.Name())
	if err != nil {
		_ = sp.Rollback(ctx)
		return fmt.Errorf("eventbus: đánh dấu đã xử lý: %w", err)
	}

	// Không chèn được nghĩa là bên nhận này ĐÃ xử lý event này.
	//
	// Đây là đường đi bình thường của mô hình at-least-once, không phải
	// tình huống bất thường.
	if tag.RowsAffected() == 0 {
		_ = sp.Rollback(ctx)
		return nil
	}

	if err := h.Handle(withCorrelation(withTx(ctx, sp), e.CorrelationID), e); err != nil {
		_ = sp.Rollback(ctx)
		metrics.HandlerFailures.WithLabelValues(h.Name(), e.Type).Inc()
		return fmt.Errorf("%s: %w", h.Name(), err)
	}

	if err := sp.Commit(ctx); err != nil {
		return fmt.Errorf("eventbus: xác nhận savepoint: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- Ngữ cảnh

type txKey struct{}

// withTx gắn giao dịch vào ngữ cảnh để handler dùng lại.
func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom lấy giao dịch mà dispatcher đang mở.
//
// BÊN NHẬN PHẢI DÙNG HÀM NÀY. Ghi bằng một kết nối khác nghĩa là việc xử
// lý và việc đánh dấu đã xử lý nằm ở hai giao dịch — và khi một cái thành
// công còn cái kia thất bại, event sẽ được xử lý hai lần.
//
// Với tiền, xử lý hai lần nghĩa là ghi sổ hai lần.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// ErrNoTx khi bên nhận gọi TxFrom ngoài ngữ cảnh dispatcher.
var ErrNoTx = errors.New("eventbus: không có giao dịch trong ngữ cảnh")

// MustTxFrom lấy giao dịch, trả lỗi nếu không có.
func MustTxFrom(ctx context.Context) (pgx.Tx, error) {
	tx, ok := TxFrom(ctx)
	if !ok {
		return nil, ErrNoTx
	}
	return tx, nil
}

// correlationKey mang correlation id của event ĐANG được xử lý.
type correlationKey struct{}

// withCorrelation gắn correlation id vào ngữ cảnh chạy của bên nhận.
//
// Nhờ vậy event mà bên nhận phát ra trong lúc xử lý sẽ KẾ THỪA cùng một
// chuỗi truy vết: một hành động của khách sinh ra cả cây event, và cả cây
// đó phải mang chung một mã để lần lại được.
func withCorrelation(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationKey{}, id)
}

// CorrelationFrom đọc correlation id của chuỗi đang chạy.
func CorrelationFrom(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}
