package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/recommendation/domain"
)

func qs(size string, k domain.Outcome, seq int64) domain.Observation {
	return domain.Observation{Size: size, Outcome: k, Seq: seq}
}

var scaleSML = []string{"S", "M", "L", "XL"}

// TestReturnOutweighsEarlierPurchase.
//
// Một lần trả hàng vì size là khách CHỦ ĐỘNG nói ra rằng size đó sai, và
// sai theo hướng nào. Một lần mua rồi im lặng thì mơ hồ: có thể vừa, có
// thể họ ngại trả. Nên phàn nàn phải thắng.
func TestReturnOutweighsEarlierPurchase(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("M", domain.OutcomePurchased, 1),
		qs("M", domain.OutcomeTight, 2),
	}, scaleSML)
	if !ok {
		t.Fatal("có phàn nàn CHẬT mà không gợi ý gì")
	}
	if got.Size != "L" {
		t.Errorf("gợi ý %q, cần L — chật thì phải LÊN một size", got.Size)
	}
	if got.LyDo != domain.ReasonReturned {
		t.Errorf("lý do %q, cần RETURN_HISTORY", got.LyDo)
	}
}

func TestLooseStepsDownOneSize(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("L", domain.OutcomeLoose, 1),
	}, scaleSML)
	if !ok || got.Size != "M" {
		t.Errorf("gợi ý %q (ok=%v), cần M — rộng thì phải XUỐNG một size",
			got.Size, ok)
	}
}

// TestKeepsPurchasedSizeWithoutComplaints — đường yếu hơn, vẫn có ích.
func TestKeepsPurchasedSizeWithoutComplaints(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("M", domain.OutcomePurchased, 1),
	}, scaleSML)
	if !ok || got.Size != "M" || got.LyDo != domain.ReasonPurchased {
		t.Errorf("gợi ý %q lý do %q (ok=%v), cần M / PREVIOUS_PURCHASE",
			got.Size, got.LyDo, ok)
	}
}

// TestLatestComplaintWins: khách đổi dáng người theo thời gian.
func TestLatestComplaintWins(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("M", domain.OutcomeTight, 1), // cũ: chật → L
		qs("L", domain.OutcomeLoose, 5), // mới: rộng → M
	}, scaleSML)
	if !ok || got.Size != "M" {
		t.Errorf("gợi ý %q, cần M — quan sát MỚI NHẤT phải thắng", got.Size)
	}
}

// TestNoAdviceWhenTightAtLargestSize.
//
// Ca dễ sai nhất. Khách nói XL vẫn chật, mà XL đã là lớn nhất scale — KHÔNG
// có size nào để gợi ý.
//
// Và tuyệt đối KHÔNG được rơi xuống "size đã mua": khách vừa nói size ấy
// sai, gợi ý lại chính nó là phớt lờ điều họ nói, rồi họ trả hàng lần nữa.
func TestNoAdviceWhenTightAtLargestSize(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("XL", domain.OutcomePurchased, 1),
		qs("XL", domain.OutcomeTight, 2),
	}, scaleSML)
	if ok {
		t.Errorf("gợi ý %q trong khi XL đã là lớn nhất scale — và đó lại "+
			"chính là size khách vừa nói là chật", got.Size)
	}
}

// TestNoAdviceForSizeOutsideScale.
//
// Khách từng mua size 38 của một mã giày; sản phẩm đang xem là áo bán theo
// S/M/L. Không có phép quy đổi nào giữa hai scale, nên không gợi ý.
func TestNoAdviceForSizeOutsideScale(t *testing.T) {
	if got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("38", domain.OutcomePurchased, 1),
	}, scaleSML); ok {
		t.Errorf("gợi ý %q cho size nằm ngoài scale đang bán", got.Size)
	}
}

func TestNoAdviceWithoutObservations(t *testing.T) {
	if _, ok := domain.SuyLuanSize(nil, scaleSML); ok {
		t.Error("gợi ý dù không có quan sát nào")
	}
}

func TestNoAdviceWithEmptyScale(t *testing.T) {
	if _, ok := domain.SuyLuanSize([]domain.Observation{
		qs("M", domain.OutcomePurchased, 1),
	}, nil); ok {
		t.Error("gợi ý dù sản phẩm không bán size nào")
	}
}

// TestPreservesCallerScaleOrder.
//
// Thang KHÔNG được sắp theo bảng chữ cái: "S, M, L" sắp chữ cái thành
// "L, M, S" và mọi phép "lên một size" đảo chiều. Bài này dùng một scale
// mà thứ tự chữ cái NGƯỢC hẳn thứ tự thật.
func TestPreservesCallerScaleOrder(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs("S", domain.OutcomeTight, 1),
	}, []string{"S", "M", "L"})
	if !ok || got.Size != "M" {
		t.Errorf("gợi ý %q, cần M — scale phải giữ đúng thứ tự bên gọi "+
			"đưa vào, không sắp lại theo bảng chữ cái", got.Size)
	}
}

// TestSizeMatchIgnoresCaseAndSpaces.
func TestSizeMatchIgnoresCaseAndSpaces(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.Observation{
		qs(" m ", domain.OutcomeTight, 1),
	}, []string{"s", "m", "l"})
	if !ok || got.Size != "L" {
		t.Errorf("gợi ý %q (ok=%v), cần L", got.Size, ok)
	}
}
