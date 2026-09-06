import { expect, test } from "@playwright/test";

import { offerBadge } from "../apps/seller/src/lib/status";

/**
 * Huy hiệu offer trong Seller Center.
 *
 * # Vì sao bài test này tồn tại
 *
 * Màn hình này từng chỉ đọc `status`, nên một offer hết sạch hàng vẫn đeo
 * huy hiệu xanh "Đang bán". Không phải vì ai đó tắt tín hiệu đi, mà vì
 * `status` KHÔNG BAO GIỜ nói được điều đó: hết hàng là sự thật của tồn kho,
 * offer hết hàng vẫn `ACTIVE` (P3-23).
 *
 * Thứ nói được là cặp (`status`, `is_sellable`) — và đó chính là thứ dễ bị
 * một lần "dọn dẹp" sau này rút gọn lại thành `status`.
 */
test("ACTIVE mà không bán được thì KHÔNG hiện là đang bán", () => {
  const b = offerBadge("ACTIVE", false);

  expect(b.label).not.toBe("Đang bán");
  expect(b.tone).toBe("warning");
});

test("ACTIVE và bán được thì hiện đang bán", () => {
  expect(offerBadge("ACTIVE", true)).toEqual({
    label: "Đang bán",
    tone: "success",
  });
});

/**
 * `is_sellable` KHÔNG lấn át trạng thái thật của offer.
 *
 * Nhà bán đã lưu trữ một offer thì cần thấy "Đã lưu trữ", không phải
 * "Không bán được" — hai câu dẫn tới hai hành động khác nhau, và câu thứ
 * hai đẩy họ đi cập nhật kho cho một offer đã ngừng vĩnh viễn.
 */
test("offer không ACTIVE giữ nguyên nhãn trạng thái của nó", () => {
  for (const [status, label] of [
    ["DRAFT", "Nháp"],
    ["SUSPENDED", "Bị tạm ngưng"],
    ["ARCHIVED", "Đã lưu trữ"],
  ] as const) {
    const b = offerBadge(status, false);
    expect(b.label).toBe(label);
    expect(b.tone).toBe("neutral");
  }
});

/**
 * Trường vắng mặt phải nghiêng về phía THẬN TRỌNG.
 *
 * `is_sellable` là bắt buộc trong đặc tả, nhưng một phản hồi cũ hoặc một
 * lỗi mạng có thể để nó `undefined`. Đoán "bán được" ở đó là hứa với nhà
 * bán một điều mình không biết.
 */
test("thiếu is_sellable thì không dám báo đang bán", () => {
  expect(offerBadge("ACTIVE", undefined).label).not.toBe("Đang bán");
});
