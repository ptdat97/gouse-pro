package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// LedgerRepository là PORT cho sổ cái.
//
// CHỈ CÓ GHI THÊM VÀ ĐỌC — không có Update, không có Delete.
//
// Sự thiếu vắng hai phương thức đó là điều quan trọng nhất của interface
// này. Nó nghĩa là KHÔNG CÓ ĐƯỜNG NÀO trong code sửa được một bút toán đã
// ghi. Ở tầng database còn có trigger chặn — hai lớp bảo vệ cho cùng một
// bất biến, vì đây là chỗ sai lầm tốn kém nhất.
type LedgerRepository interface {
	// Append ghi một bút toán.
	//
	// Trả ErrDuplicateEntry nếu khóa idempotency đã dùng. Bên gọi nên coi
	// đó là THÀNH CÔNG và đọc lại bút toán cũ — ghi hai lần cùng một sự
	// kiện tài chính sẽ nhân đôi số tiền.
	Append(ctx context.Context, e *LedgerEntry) error

	FindByID(ctx context.Context, id ids.ID) (*LedgerEntry, error)

	// FindByIdempotencyKey tra bút toán đã ghi theo khóa.
	//
	// Dùng để trả kết quả cũ khi bên gọi thử lại — đó là ý nghĩa của
	// idempotent: gọi nhiều lần cho cùng một kết quả.
	FindByIdempotencyKey(ctx context.Context, key string) (*LedgerEntry, error)

	// FindByReference lấy mọi bút toán của một nguồn gốc.
	//
	// Trả lời "đơn hàng này đã ghi sổ những gì" — câu hỏi đầu tiên khi có
	// tranh chấp.
	FindByReference(ctx context.Context, refType string, refID ids.ID) ([]*LedgerEntry, error)

	// FindAll lấy bút toán trong một khoảng thời gian, cho job đối soát.
	FindAll(ctx context.Context, from, to time.Time, limit int) ([]*LedgerEntry, error)
}

// BalanceRepository là PORT cho việc tính số dư.
//
// Tách khỏi LedgerRepository vì đây là thao tác ĐỌC TỔNG HỢP, cài đặt bằng
// truy vấn gom nhóm chứ không phải đọc từng bút toán.
type BalanceRepository interface {
	// Balance tính số dư của một tài khoản.
	//
	// KẾT QUẢ TÍNH TOÁN từ bút toán, không phải trường được lưu.
	Balance(ctx context.Context, account Account) (Balance, error)

	// BalancesByOwner tính số dư mọi tài khoản của một chủ sở hữu.
	//
	// Seller hỏi "tôi còn bao nhiêu tiền" thì gọi hàm này.
	BalancesByOwner(ctx context.Context, ownerID ids.ID) (map[string]Balance, error)

	// TotalDebitCredit trả tổng ghi nợ và ghi có TOÀN HỆ THỐNG.
	//
	// Dùng cho job đối soát hàng ngày: hai con số này phải BẰNG NHAU. Lệch
	// nghĩa là có bút toán không cân bằng lọt vào database — sự cố nghiêm
	// trọng, không phải "sai số chấp nhận được".
	TotalDebitCredit(ctx context.Context) (debit, credit int64, err error)
}
