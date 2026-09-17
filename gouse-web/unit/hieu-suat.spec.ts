import { expect, test } from "@playwright/test";

import {
  chiSoGiaTri,
  chiSoNguong,
  chiSoNhan,
  chiSoTone,
  chiSoTrangThaiNhan,
} from "../apps/seller/src/lib/hieu-suat";

/**
 * Màn hình hiệu suất tồn tại để KHÔNG phải là hộp đen. Ba bài đầu giữ ba
 * chỗ mà một con số hiện sai sẽ nói ngược hẳn về việc nhà bán có đạt hay
 * không.
 */

test("tỷ lệ hiện bằng PHẦN TRĂM, không phải số thô", () => {
  /**
   * API trả `0.03` cho một thứ nghĩa là 3%. Hiện thẳng `0.03` là sai ở mức
   * người đọc không nhận ra — con số trông hoàn toàn hợp lệ, chỉ nhỏ hơn
   * sự thật một trăm lần.
   */
  expect(chiSoGiaTri("cancellation_rate", 0.03)).toBe("3%");
  expect(chiSoGiaTri("on_time_shipping_rate", 0.955)).toBe("95,5%");
  expect(chiSoGiaTri("cancellation_rate", 0)).toBe("0%");
});

test("ngưỡng phải nói RÕ HƯỚNG: ≤ hay ≥", () => {
  /**
   * "Ngưỡng 3%" một mình không nói được gì — 3% là sàn hay trần? Với tỷ lệ
   * hủy thì càng thấp càng tốt, với giao đúng hạn thì ngược lại. Hiện sai
   * hướng là nói với nhà bán rằng họ đang đạt trong khi họ đang vi phạm.
   */
  expect(chiSoNguong("cancellation_rate", 0.03)).toBe("≤ 3%");
  expect(chiSoNguong("on_time_shipping_rate", 0.95)).toBe("≥ 95%");
});

test("chỉ số LẠ không bị đoán đơn vị và không thành ô trống", () => {
  /**
   * Suy đơn vị từ hậu tố `_rate` chạy được hôm nay và hỏng im lặng vào
   * ngày có chỉ số đầu tiên không phải tỷ lệ. Chỉ số lạ phải LỘ RA.
   */
  expect(chiSoNhan("chi_so_moi_toanh")).toBe("chi_so_moi_toanh");
  expect(chiSoGiaTri("chi_so_moi_toanh", 42)).toBe("42");
  expect(chiSoNguong("chi_so_moi_toanh", 42)).toBe("ngưỡng 42");
  expect(chiSoNhan(undefined)).toBe("—");
  expect(chiSoGiaTri("cancellation_rate", undefined)).toBe("—");
});

test("mọi chỉ số backend có thể trả đều có nhãn tiếng Việt", () => {
  // Bốn cái cuối chỉ xuất hiện trong `not_measured`, nhưng vẫn hiện ra
  // màn hình — mã thô `buy_box_win_rate` là bắt nhà bán tự đoán.
  for (const ten of [
    "cancellation_rate",
    "on_time_shipping_rate",
    "return_rate_description",
    "average_rating",
    "inventory_accuracy",
    "buy_box_win_rate",
  ]) {
    expect(chiSoNhan(ten), `mã ${ten} chưa có nhãn`).not.toBe(ten);
  }
});

test("CRITICAL là cảnh báo, không phải lời chê", () => {
  // Chữ này đi kèm hậu quả thật (mất ô mua, bị tạm ngưng), nên nó phải
  // nghe như một ngưỡng bị vượt chứ không như một điểm số kém.
  expect(chiSoTrangThaiNhan("CRITICAL")).toContain("ngưỡng");
  expect(chiSoTrangThaiNhan("CRITICAL").toLowerCase()).not.toContain("kém");
  expect(chiSoTone("CRITICAL")).toBe("danger");
  expect(chiSoTone("GOOD")).toBe("success");
  expect(chiSoTone("WARNING")).toBe("warning");
});
