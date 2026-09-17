import { expect, test } from "@playwright/test";

import {
  returnReasonLabel,
  returnStatusLabel,
  returnTone,
} from "../apps/seller/src/lib/status";

/**
 * Nhãn của luồng trả hàng.
 *
 * Trả hàng là chỗ khách đang bực và nhà bán đang mất tiền, nên mỗi chữ
 * trên màn hình phải nói đúng thứ vừa xảy ra. Ba bài dưới đây giữ ba chỗ
 * dễ nói sai nhất.
 */

test("mọi mã lý do của backend đều có nhãn tiếng Việt", () => {
  /**
   * Danh sách lấy từ `ReturnReasonCode` trong đặc tả, và đặc tả khớp
   * `domain.LyDo` của module returns.
   *
   * Thiếu một mã thì màn hình hiện thẳng chuỗi `WRONG_ITEM_SENT` cho
   * người trực — đọc được, nhưng nó nói rằng phần còn lại của trang cũng
   * có thể chưa ai xem qua.
   *
   * Bài này từng đỏ thật: bản đầu viết `WRONG_ITEM` (backend dùng
   * `WRONG_ITEM_SENT`) và bỏ sót ba mã.
   */
  const ma = [
    "SIZE_TOO_SMALL",
    "SIZE_TOO_LARGE",
    "NOT_AS_DESCRIBED",
    "COLOR_DIFFERENT",
    "QUALITY_ISSUE",
    "DEFECTIVE",
    "DAMAGED_IN_TRANSIT",
    "WRONG_ITEM_SENT",
    "CHANGED_MIND",
    "LATE_DELIVERY",
  ];

  for (const m of ma) {
    const nhan = returnReasonLabel(m);
    expect(nhan, `mã ${m} chưa có nhãn`).not.toBe(m);
    expect(nhan.trim()).not.toBe("");
  }
});

test("mã lạ KHÔNG làm crash và KHÔNG thành ô trống", () => {
  // Yêu cầu bắt buộc của openapi.yaml: enum lạ không được làm hỏng giao
  // diện. Nhưng "không hỏng" mà hiện khoảng trắng thì người trực không
  // biết khách than gì — tệ theo kiểu khó phát hiện hơn.
  expect(returnReasonLabel("MA_MOI_CHUA_BIET")).toBe("MA_MOI_CHUA_BIET");
  expect(returnReasonLabel(undefined)).toBe("—");
  expect(returnStatusLabel("TRANG_THAI_LA")).toBe("TRANG_THAI_LA");
});

test("REFUNDED nói về TIỀN, không nói hàng đã xong", () => {
  /**
   * Chỗ dễ nói sai nhất của cả màn hình.
   *
   * `REFUNDED` nghĩa là tiền đã về tay khách. Món hàng thì vẫn nằm ở
   * `Returned` và KHÔNG bán lại được cho tới khi có người kiểm định.
   *
   * Gọi nó là "Hoàn tất" sẽ khiến nhà bán bỏ qua bước kiểm định — và khi
   * ấy họ mất cả tiền lẫn hàng, vì hàng chưa kiểm nằm chết trong kho.
   */
  const nhan = returnStatusLabel("REFUNDED");
  expect(nhan).toContain("hoàn tiền");
  expect(nhan.toLowerCase()).not.toContain("hoàn tất");
});

test("việc CẦN LÀM nổi bật hơn việc đã xong", () => {
  // Một yêu cầu chờ duyệt để lâu là khách ngồi chờ trong lúc đang bực.
  // Nó phải khác màu với những dòng không cần ai làm gì.
  expect(returnTone("REQUESTED")).toBe("warning");
  expect(returnTone("REFUNDED")).toBe("success");
  expect(returnTone("REJECTED")).toBe("danger");
  expect(returnTone("CANCELLED")).toBe("danger");
});
