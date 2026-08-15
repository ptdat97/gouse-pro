package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// searchSignalPublisher nối cổng ra của tầng application với outbox.
type searchSignalPublisher struct {
	outbox *eventbus.Outbox
}

var _ application.SearchSignalPublisher = (*searchSignalPublisher)(nil)

// NewSearchSignalPublisher tạo bộ phát tín hiệu tìm kiếm.
func NewSearchSignalPublisher(outbox *eventbus.Outbox) application.SearchSignalPublisher {
	return &searchSignalPublisher{outbox: outbox}
}

// PublishSearchNoResult ghi event "khách tìm mà không ra kết quả".
//
// # Phát RỜI, không nằm trong giao dịch nào
//
// Khác với event thêm giỏ hay đặt hàng, ở đây KHÔNG có thay đổi nghiệp vụ
// để gắn vào — tìm kiếm là thao tác chỉ đọc. Vì thế dùng `Publish` chứ
// không phải `PublishTx`.
//
// Đánh đổi: mất một tín hiệu nếu tiến trình chết đúng lúc. Chấp nhận được
// vì tín hiệu tìm kiếm mang tính THỐNG KÊ — thiếu một bản ghi không làm
// sai kết luận, khác hẳn sổ cái hay vết kiểm toán.
func (p *searchSignalPublisher) PublishSearchNoResult(
	ctx context.Context, query string,
) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeSearchNoResult,
		eventbus.AggregateSearch,
		// Tìm kiếm không có thực thể nào, nên định danh là BẢN BĂM của từ
		// khóa đã chuẩn hóa. Nhờ vậy cùng một từ khóa luôn cho cùng định
		// danh, và event trùng lặp phát hiện được.
		searchID(query),
		struct {
			Query string `json:"query"`
		}{Query: query},
	)
	if err != nil {
		return err
	}

	return p.outbox.Publish(ctx, e)
}

// searchID sinh định danh ổn định cho một từ khóa.
//
// Chuẩn hóa về chữ thường trước khi băm: "Áo Sơ Mi" và "áo sơ mi" là cùng
// một nhu cầu, và đếm chúng thành hai dòng khác nhau sẽ làm loãng tín hiệu.
func searchID(query string) ids.ID {
	sum := sha256.Sum256([]byte(strings.ToLower(query)))
	return ids.ID("sch_" + hex.EncodeToString(sum[:13]))
}
