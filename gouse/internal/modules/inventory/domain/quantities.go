// Package domain chứa mô hình nghiệp vụ của module inventory.
//
// Module này trả lời đúng một câu hỏi: "còn bao nhiêu, ở đâu, trạng thái
// gì". Nó KHÔNG biết về đơn hàng, khách hàng, hay lý do nghiệp vụ — đó là
// việc của module gọi nó.
package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNegativeQuantity  = errors.New("inventory: số lượng không được âm")
	ErrInsufficientStock = errors.New("inventory: không đủ hàng")
	ErrInvariantBroken   = errors.New("inventory: tổng các trạng thái không khớp số lượng vật lý")
)

// Quantities là số lượng hàng theo SÁU trạng thái.
//
// Đây là VALUE OBJECT: bất biến, mọi phép biến đổi trả về bản mới. Nhờ vậy
// một phép tính thất bại không để lại trạng thái dở dang.
//
// BẤT BIẾN CỐT LÕI (quy tắc 1 và 2, mục 12 của đặc tả):
//
//	available + reserved + committed + in_transit + damaged + returned
//	    = tổng số lượng vật lý
//	Mọi thành phần ≥ 0
//
// Vi phạm bất biến này dẫn tới bán hàng không có, hoặc hàng bị khóa vĩnh
// viễn. Đó là lý do mọi phép biến đổi ở đây đều CHUYỂN số lượng giữa các
// trạng thái chứ không cộng/trừ tùy tiện — tổng luôn được bảo toàn theo
// cấu trúc, không phải nhờ người viết nhớ kiểm tra.
type Quantities struct {
	available int
	reserved  int
	committed int
	inTransit int
	damaged   int
	returned  int
}

// NewQuantities tạo bộ số lượng, kiểm tra không có thành phần âm.
func NewQuantities(available, reserved, committed, inTransit, damaged, returned int) (Quantities, error) {
	q := Quantities{
		available: available,
		reserved:  reserved,
		committed: committed,
		inTransit: inTransit,
		damaged:   damaged,
		returned:  returned,
	}
	if err := q.validate(); err != nil {
		return Quantities{}, err
	}
	return q, nil
}

// Empty là bộ số lượng rỗng — điểm khởi đầu của mọi InventoryItem.
func Empty() Quantities { return Quantities{} }

func (q Quantities) validate() error {
	for name, v := range map[string]int{
		"available": q.available, "reserved": q.reserved, "committed": q.committed,
		"in_transit": q.inTransit, "damaged": q.damaged, "returned": q.returned,
	} {
		if v < 0 {
			return fmt.Errorf("%w: %s = %d", ErrNegativeQuantity, name, v)
		}
	}
	return nil
}

func (q Quantities) Available() int { return q.available }
func (q Quantities) Reserved() int  { return q.reserved }
func (q Quantities) Committed() int { return q.committed }
func (q Quantities) InTransit() int { return q.inTransit }
func (q Quantities) Damaged() int   { return q.damaged }
func (q Quantities) Returned() int  { return q.returned }

// Total là tổng số lượng VẬT LÝ đang nắm giữ.
//
// Lưu ý: hàng đã xuất (Ship) KHÔNG còn nằm trong đây — nó đã rời khỏi tồn kho.
func (q Quantities) Total() int {
	return q.available + q.reserved + q.committed + q.inTransit + q.damaged + q.returned
}

// IsDepleted cho biết đã hết hàng bán được chưa.
//
// Chỉ xét `available`: hàng đang giữ cho checkout khác, hàng hỏng, hàng
// chờ kiểm định đều KHÔNG bán được cho khách mới.
func (q Quantities) IsDepleted() bool { return q.available == 0 }

// IsLowStock cho biết đã xuống dưới ngưỡng cảnh báo chưa.
func (q Quantities) IsLowStock(threshold int) bool {
	return q.available <= threshold
}

// ---------------------------------------------------------------- Chuyển đổi

// state định danh một trong sáu trạng thái tồn kho.
//
// Dùng kiểu liệt kê thay vì con trỏ trường: con trỏ vào trường của struct
// giá trị rất dễ trỏ nhầm vào bản gốc thay vì bản sao, và lỗi đó âm thầm
// làm hỏng bất biến.
type state int

const (
	stAvailable state = iota
	stReserved
	stCommitted
	stInTransit
	stDamaged
	stReturned
)

func (s state) String() string {
	switch s {
	case stAvailable:
		return "available"
	case stReserved:
		return "reserved"
	case stCommitted:
		return "committed"
	case stInTransit:
		return "in_transit"
	case stDamaged:
		return "damaged"
	case stReturned:
		return "returned"
	}
	return "unknown"
}

func (q *Quantities) field(s state) *int {
	switch s {
	case stAvailable:
		return &q.available
	case stReserved:
		return &q.reserved
	case stCommitted:
		return &q.committed
	case stInTransit:
		return &q.inTransit
	case stDamaged:
		return &q.damaged
	case stReturned:
		return &q.returned
	}
	return nil
}

// move chuyển `qty` từ trạng thái này sang trạng thái khác.
//
// Đây là phép toán CƠ SỞ của cả module: mọi chuyển đổi trạng thái đều đi
// qua đây, nên tổng số lượng vật lý được bảo toàn theo CẤU TRÚC — không có
// đường nào để một phép chuyển làm tổng thay đổi.
//
// Thao tác trên BẢN SAO rồi mới trả về: một phép chuyển thất bại không để
// lại trạng thái dở dang.
func (q Quantities) move(qty int, from, to state) (Quantities, error) {
	if qty <= 0 {
		return q, fmt.Errorf("inventory: số lượng chuyển phải lớn hơn 0, nhận %d", qty)
	}

	out := q
	src, dst := out.field(from), out.field(to)

	if *src < qty {
		return q, fmt.Errorf("%w: %s có %d, cần %d", ErrInsufficientStock, from, *src, qty)
	}
	*src -= qty
	*dst += qty
	return out, nil
}

// Reserve giữ hàng cho một checkout: Available → Reserved.
func (q Quantities) Reserve(qty int) (Quantities, error) {
	return q.move(qty, stAvailable, stReserved)
}

// Release giải phóng hàng đang giữ: Reserved → Available.
//
// Dùng khi khách hủy checkout hoặc reservation hết hạn.
func (q Quantities) Release(qty int) (Quantities, error) {
	return q.move(qty, stReserved, stAvailable)
}

// Commit cam kết hàng cho đơn đã xác nhận: Reserved → Committed.
func (q Quantities) Commit(qty int) (Quantities, error) {
	return q.move(qty, stReserved, stCommitted)
}

// Uncommit hoàn tác cam kết: Committed → Available.
//
// Trường hợp hiếm: hủy đơn TRƯỚC khi xuất hàng. Trả thẳng về Available
// chứ không về Reserved vì checkout đã kết thúc, không còn ai giữ chỗ.
func (q Quantities) Uncommit(qty int) (Quantities, error) {
	return q.move(qty, stCommitted, stAvailable)
}

// Ship xuất hàng: Committed → RỜI KHỎI tồn kho.
//
// Đây là phép DUY NHẤT làm giảm tổng số lượng vật lý, và điều đó đúng:
// hàng đã ra khỏi kho, không còn là tồn kho nữa.
func (q Quantities) Ship(qty int) (Quantities, error) {
	if qty <= 0 {
		return q, fmt.Errorf("inventory: số lượng xuất phải lớn hơn 0, nhận %d", qty)
	}
	if q.committed < qty {
		return q, fmt.Errorf("%w: committed có %d, cần %d", ErrInsufficientStock, q.committed, qty)
	}
	out := q
	out.committed -= qty
	return out, nil
}

// Receive nhập hàng mới vào kho: TĂNG Available.
//
// Cùng với Ship, đây là hai phép làm đổi tổng số lượng vật lý — vì hàng
// thật sự vào hoặc ra khỏi kho.
func (q Quantities) Receive(qty int) (Quantities, error) {
	if qty <= 0 {
		return q, fmt.Errorf("inventory: số lượng nhập phải lớn hơn 0, nhận %d", qty)
	}
	out := q
	out.available += qty
	return out, nil
}

// ReceiveReturn nhận hàng khách trả về: TĂNG Returned.
//
// QUY TẮC 3 (mục 12): hàng hoàn KHÔNG BAO GIỜ tự động cộng vào Available.
// Nó vào trạng thái Returned và phải qua kiểm định.
//
// Vi phạm quy tắc này dẫn tới bán lại hàng hỏng cho khách khác — thiệt hại
// uy tín lớn hơn nhiều so với giá trị món hàng. Đó là lý do KHÔNG có hàm
// nào cho phép đi thẳng từ hàng hoàn vào Available.
func (q Quantities) ReceiveReturn(qty int) (Quantities, error) {
	if qty <= 0 {
		return q, fmt.Errorf("inventory: số lượng hoàn phải lớn hơn 0, nhận %d", qty)
	}
	out := q
	out.returned += qty
	return out, nil
}

// InspectionPassed hàng hoàn đạt kiểm định: Returned → Available.
func (q Quantities) InspectionPassed(qty int) (Quantities, error) {
	return q.move(qty, stReturned, stAvailable)
}

// InspectionFailed hàng hoàn không đạt: Returned → Damaged.
func (q Quantities) InspectionFailed(qty int) (Quantities, error) {
	return q.move(qty, stReturned, stDamaged)
}

// MarkDamaged đánh dấu hàng khả dụng bị hỏng: Available → Damaged.
func (q Quantities) MarkDamaged(qty int) (Quantities, error) {
	return q.move(qty, stAvailable, stDamaged)
}

// SendInTransit chuyển hàng đi kho khác: Available → In Transit.
func (q Quantities) SendInTransit(qty int) (Quantities, error) {
	return q.move(qty, stAvailable, stInTransit)
}

// ArriveFromTransit hàng trung chuyển về tới nơi: In Transit → Available.
func (q Quantities) ArriveFromTransit(qty int) (Quantities, error) {
	return q.move(qty, stInTransit, stAvailable)
}

// AdjustAvailable điều chỉnh thủ công số lượng khả dụng (kiểm kê).
//
// delta có thể ÂM. Đây là phép duy nhất cho phép đặt số lượng tùy ý, nên
// quy tắc 7 (mục 12) yêu cầu mọi lần gọi phải kèm lý do và người thực hiện
// — việc đó được cưỡng chế ở tầng application, không phải ở đây.
func (q Quantities) AdjustAvailable(delta int) (Quantities, error) {
	if delta == 0 {
		return q, errors.New("inventory: điều chỉnh bằng 0 không có tác dụng")
	}
	out := q
	out.available += delta
	if out.available < 0 {
		return q, fmt.Errorf("%w: điều chỉnh %d làm available thành %d",
			ErrNegativeQuantity, delta, out.available)
	}
	return out, nil
}
