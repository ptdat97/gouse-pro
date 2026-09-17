import { expect, test } from "@playwright/test";

import {
  settlementStatusLabel,
  settlementTone,
} from "../apps/seller/src/lib/status";

/**
 * Nhãn của màn hình TIỀN.
 *
 * Đây là màn hình nhà bán mở ra để biết bao giờ nhận được tiền. Mỗi chữ ở
 * đây trả lời một câu hỏi về tiền, nên chữ sai không chỉ khó đọc — nó trả
 * lời sai.
 */

test("ba trạng thái của backend đều có nhãn tiếng Việt", () => {
  /**
   * BA, không phải bốn. `PENDING_CONFIRMATION` từng nằm trong đặc tả mà
   * chưa bao giờ có trong `domain.TrangThaiDoiSoat` — nếu có ai thêm lại,
   * bài này không bắt được, nhưng `apicheck` và đặc tả nay đã khớp mã.
   */
  for (const ma of ["DRAFT", "CONFIRMED", "PAID"]) {
    const nhan = settlementStatusLabel(ma);
    expect(nhan, `mã ${ma} chưa có nhãn`).not.toBe(ma);
    expect(nhan.trim()).not.toBe("");
  }
});

test("DRAFT nói về TIỀN đang chờ duyệt, không phải về một bản ghi viết dở", () => {
  /**
   * Chỗ dễ nói sai nhất.
   *
   * Dịch thẳng `DRAFT` thành "Nháp" là đúng từ điển và sai nghiệp vụ: với
   * nhà bán, đợt này đã CHỐT SỐ và đang chờ nền tảng duyệt chi. Gọi là
   * "Nháp" khiến họ tưởng con số còn thay đổi được và không đối chiếu.
   */
  const nhan = settlementStatusLabel("DRAFT");
  expect(nhan.toLowerCase()).not.toContain("nháp");
  expect(nhan.toLowerCase()).toContain("chờ");
});

test("chỉ ĐÃ CHUYỂN TIỀN mới là success", () => {
  // Tô xanh một đợt chưa chuyển tiền là nói với nhà bán rằng tiền đã về.
  expect(settlementTone("PAID")).toBe("success");
  expect(settlementTone("CONFIRMED")).not.toBe("success");
  expect(settlementTone("DRAFT")).not.toBe("success");
});

test("mã lạ KHÔNG làm crash và KHÔNG thành ô trống", () => {
  expect(settlementStatusLabel("TRANG_THAI_LA")).toBe("TRANG_THAI_LA");
  expect(settlementStatusLabel(undefined)).toBe("—");
});
