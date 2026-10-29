package domain_test

import (
	"testing"
	"time"
)

const lostContactThreshold = 168 * time.Hour // mặc định của fulfillment.delivery_silence_hours

// TestNotYetHandedOverIsNotLostContact — gói còn trong tay nhà bán không mất tin.
//
// Việc đó do `PastDeadline` trông. Lẫn hai thứ sẽ báo "mất tin vận chuyển" cho
// một gói chưa hề rời kho, và người trực sẽ đi hỏi hãng vận chuyển về một
// mã vận đơn chưa tồn tại.
func TestNotYetHandedOverIsNotLostContact(t *testing.T) {
	fo := foNew(t)
	rat := fo.CreatedAt().Add(365 * 24 * time.Hour)

	if fo.LostContact(lostContactThreshold, rat) {
		t.Error("đơn CHƯA bàn giao bị coi là mất tin vận chuyển")
	}
}

// TestBelowTheThresholdIsNotLostContact — im lặng ngắn là bình thường.
func TestBelowTheThresholdIsNotLostContact(t *testing.T) {
	fo := foHandedOver(t, time.Hour)

	// Ngay TRƯỚC ngưỡng: gói vẫn đang đi bình thường.
	before := fo.ShippedAt().Add(lostContactThreshold - time.Minute)
	if fo.LostContact(lostContactThreshold, before) {
		t.Error("gói im lặng chưa tới ngưỡng đã bị gọi là mất tin")
	}
}

// TestPastTheThresholdIsLostContact — đây là ca job phải bắt được.
func TestPastTheThresholdIsLostContact(t *testing.T) {
	fo := foHandedOver(t, time.Hour)

	after := fo.ShippedAt().Add(lostContactThreshold + time.Minute)
	if !fo.LostContact(lostContactThreshold, after) {
		t.Errorf("gói bàn giao lúc %v, im lặng quá %v mà không bị coi là mất tin",
			fo.ShippedAt(), lostContactThreshold)
	}
}

// TestOnceDeliveredItIsNoLongerLostContact — tin đã về thì hết chuyện.
//
// Không có bài này thì một gói đã giao xong vẫn nằm mãi trong danh sách
// cảnh báo, và danh sách chỉ tăng cho tới lúc không ai đọc nữa.
func TestOnceDeliveredItIsNoLongerLostContact(t *testing.T) {
	fo := foHandedOver(t, time.Hour)
	shippedAt := fo.ShippedAt()

	if err := fo.MarkInTransit(shippedAt.Add(time.Hour)); err != nil {
		t.Fatalf("MarkInTransit: %v", err)
	}
	if err := fo.Deliver(shippedAt.Add(2 * time.Hour)); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	rat := shippedAt.Add(365 * 24 * time.Hour)
	if fo.LostContact(lostContactThreshold, rat) {
		t.Error("gói ĐÃ GIAO vẫn bị coi là mất tin")
	}
}

// TestADeliveryFailureIsNotLostContact — tin xấu vẫn là tin.
//
// DELIVERY_FAILED nghĩa là hãng vận chuyển ĐÃ báo về. Đơn cần người xử lý,
// nhưng không phải bằng cách đi hỏi "gói này đâu rồi" — nên nó không thuộc
// danh sách này. Gộp vào sẽ pha loãng đúng thứ danh sách muốn nói.
func TestADeliveryFailureIsNotLostContact(t *testing.T) {
	fo := foHandedOver(t, time.Hour)
	shippedAt := fo.ShippedAt()

	// DELIVERY_FAILED chỉ tới được từ IN_TRANSIT — máy trạng thái không
	// cho nhảy thẳng từ HANDED_OVER, vì "giao thất bại" hàm ý đã có người
	// mang hàng đi giao.
	if err := fo.MarkInTransit(shippedAt.Add(time.Hour)); err != nil {
		t.Fatalf("MarkInTransit: %v", err)
	}
	if err := fo.MarkDeliveryFailed("khách không nghe máy", shippedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("MarkDeliveryFailed: %v", err)
	}

	rat := shippedAt.Add(365 * 24 * time.Hour)
	if fo.LostContact(lostContactThreshold, rat) {
		t.Error("gói GIAO THẤT BẠI bị gộp vào danh sách mất tin")
	}
	if fo.InTransit() {
		t.Error("DELIVERY_FAILED không còn là đang trên đường")
	}
}

// TestImLangTinhTuTinCUOI, không phải từ lúc bàn giao.
//
// Một gói đã qua IN_TRANSIT hôm qua có tin mới hơn lúc bàn giao. Tính từ
// lúc bàn giao sẽ làm nó trông đáng ngờ hơn thực tế, và người trực đi hỏi
// về một gói đang chạy bình thường.
func TestSilenceIsMeasuredFromTheLastUpdate(t *testing.T) {
	fo := foHandedOver(t, time.Hour)
	handOver := fo.ShippedAt()

	if err := fo.MarkInTransit(handOver.Add(72 * time.Hour)); err != nil {
		t.Fatalf("MarkInTransit: %v", err)
	}

	now := handOver.Add(80 * time.Hour)
	silentFor := fo.SilentFor(now)

	if silentFor != 8*time.Hour {
		t.Errorf("im lặng = %v, mong 8h (tính từ IN_TRANSIT, không phải từ bàn giao %v)",
			silentFor, 80*time.Hour)
	}
}
