package domain

import (
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// Balance là số dư của một tài khoản — KẾT QUẢ TÍNH TOÁN, không phải trường
// được cập nhật.
//
// VÌ SAO KHÔNG LƯU SỐ DƯ (ADR-0008, quyết định 3):
//
//	Nếu lưu số dư và cập nhật mỗi giao dịch:
//	  - Có thể LỆCH với tổng bút toán (do lỗi, do tranh chấp đồng thời)
//	  - Không biết lệch từ lúc nào
//	  - Trở thành ĐIỂM NGHẼN GHI: mọi giao dịch cập nhật cùng một dòng
//	  - Không tái dựng được số dư tại thời điểm quá khứ
//
// Với hệ thống GIỮ TIỀN HỘ, không có cách nào biết số dư đúng hay sai là
// rủi ro không chấp nhận được.
type Balance struct {
	Account Account

	// Amount là số dư theo bản chất tài khoản:
	//
	//	Tài khoản ghi nợ (tài sản, chi phí): Σ DEBIT − Σ CREDIT
	//	Tài khoản ghi có (nợ phải trả, doanh thu): Σ CREDIT − Σ DEBIT
	//
	// Nhờ quy ước này, số dư dương luôn có nghĩa "bình thường": nền tảng
	// đang giữ bao nhiêu tiền mặt, hoặc còn nợ seller bao nhiêu.
	Amount money.Money

	// TotalDebit và TotalCredit giữ lại để đối chiếu khi có tranh chấp.
	//
	// Seller hỏi "vì sao số dư của tôi là X" thì trả lời được: đã ghi có
	// bấy nhiêu, đã ghi nợ (chi trả) bấy nhiêu.
	TotalDebit  money.Money
	TotalCredit money.Money

	// EntryCount là số bút toán đã tính vào số dư này.
	EntryCount int
}

// ComputeBalances tính số dư của MỌI tài khoản từ danh sách bút toán.
//
// Đây là cài đặt tham chiếu: đơn giản, luôn đúng, dùng để đối chiếu với
// kết quả truy vấn tối ưu ở tầng database. Nếu hai bên ra số khác nhau thì
// truy vấn tối ưu sai.
func ComputeBalances(entries []*LedgerEntry) map[string]Balance {
	type acc struct {
		account  Account
		debit    int64
		credit   int64
		currency money.Currency
		entries  int
	}

	totals := map[string]*acc{}

	for _, e := range entries {
		if e == nil {
			continue
		}
		seen := map[string]bool{}

		for _, l := range e.Lines() {
			key := l.Account.Key()
			a, ok := totals[key]
			if !ok {
				a = &acc{account: l.Account, currency: l.Amount.Currency()}
				totals[key] = a
			}
			if l.Direction == Debit {
				a.debit += l.Amount.Amount()
			} else {
				a.credit += l.Amount.Amount()
			}
			// Một bút toán có thể có nhiều dòng cùng tài khoản; chỉ đếm một lần.
			if !seen[key] {
				a.entries++
				seen[key] = true
			}
		}
	}

	out := make(map[string]Balance, len(totals))
	for key, a := range totals {
		net := a.credit - a.debit
		if a.account.Type.IsDebitNormal() {
			net = a.debit - a.credit
		}
		out[key] = Balance{
			Account:     a.account,
			Amount:      money.MustNew(net, a.currency),
			TotalDebit:  money.MustNew(a.debit, a.currency),
			TotalCredit: money.MustNew(a.credit, a.currency),
			EntryCount:  a.entries,
		}
	}
	return out
}

// CheckIntegrity kiểm tra tính toàn vẹn của một tập bút toán.
//
// Ba chỉ số phải bằng 0 hàng ngày (deliverables.md mục 12.7). Đây là hàm
// trả lời câu hỏi thứ hai: "có bút toán nào không cân bằng không".
//
// Chạy hàng ngày. Kết quả khác 0 là sự cố NGHIÊM TRỌNG, không phải "sai số
// chấp nhận được".
type IntegrityReport struct {
	// TotalEntries là số bút toán đã kiểm tra.
	TotalEntries int

	// UnbalancedEntries liệt kê bút toán vi phạm Σ DEBIT = Σ CREDIT.
	//
	// PHẢI LUÔN RỖNG. Có phần tử nghĩa là dữ liệu đã bị can thiệp bất
	// thường — constructor không cho phép tạo bút toán lệch.
	UnbalancedEntries []string
}

// IsHealthy cho biết sổ sách có toàn vẹn không.
func (r IntegrityReport) IsHealthy() bool { return len(r.UnbalancedEntries) == 0 }

// CheckIntegrity rà soát toàn bộ bút toán.
func CheckIntegrity(entries []*LedgerEntry) IntegrityReport {
	report := IntegrityReport{TotalEntries: len(entries)}
	for _, e := range entries {
		if e == nil {
			continue
		}
		if !e.IsBalanced() {
			report.UnbalancedEntries = append(report.UnbalancedEntries, e.ID().String())
		}
	}
	return report
}
