import { expect, test } from "@playwright/test";

import {
  docMauTuURL,
  NHOM_MAU,
  nhomMauHienThi,
} from "../apps/storefront/src/lib/mau";

/**
 * Bộ lọc màu của cửa hàng.
 *
 * # Vì sao bộ bài này nhỏ
 *
 * Phần lớn hàng rào ở đây do TRÌNH BIÊN DỊCH giữ, không phải test:
 * `NHOM_MAU` khai là `Record<NhomMau, …>`, nên thêm một nhóm vào đặc tả mà
 * quên nhãn là `npm run typecheck` đỏ. Viết thêm một bài "mọi nhóm đều có
 * nhãn" sẽ là một danh sách CHÉP TAY thứ hai — đúng thứ vừa gây ra lệch
 * GRAY/GREY ở P3-70.
 *
 * Còn lại đúng một chỗ kiểu không với tới: dữ liệu từ THANH ĐỊA CHỈ.
 */

test("nhóm màu lạ trong URL bị BỎ, không gửi lên máy chủ", () => {
  // `?color=CAM_VANG` là thứ ai cũng gõ được. Gửi thẳng lên thì máy chủ
  // trả rỗng và khách thấy "không có sản phẩm nào" mà không hiểu vì sao.
  expect(docMauTuURL("CAM_VANG")).toEqual([]);
  expect(docMauTuURL("BLACK,CAM_VANG,WHITE")).toEqual(["BLACK", "WHITE"]);
});

test("URL rỗng hay thiếu KHÔNG thành một bộ lọc", () => {
  // Mảng rỗng khác mảng có một phần tử rỗng: cái sau làm máy chủ lọc theo
  // chuỗi rỗng và trả về trắng trơn.
  expect(docMauTuURL(null)).toEqual([]);
  expect(docMauTuURL("")).toEqual([]);
  expect(docMauTuURL(",,")).toEqual([]);
  expect(docMauTuURL("BLACK,,WHITE")).toEqual(["BLACK", "WHITE"]);
});

test("chữ thường và khoảng trắng vẫn đọc được", () => {
  // Người ta gõ tay vào thanh địa chỉ, và link cũ có thể viết kiểu khác.
  expect(docMauTuURL("black")).toEqual(["BLACK"]);
  expect(docMauTuURL(" BLACK , white ")).toEqual(["BLACK", "WHITE"]);
});

test("GRAY KHÔNG được nhận — máy chủ lưu GREY", () => {
  /**
   * Đây là lỗi thật của P3-70: đặc tả nói `GRAY`, máy chủ lưu `GREY`, và
   * bộ lọc im lặng trả rỗng.
   *
   * Bài này giữ cho một lần "sửa cho thân thiện" trong tương lai không
   * lặng lẽ nhận lại `GRAY` rồi gửi lên máy chủ — nơi nó không khớp gì.
   */
  expect(docMauTuURL("GRAY")).toEqual([]);
  expect(docMauTuURL("GREY")).toEqual(["GREY"]);
});

test("mọi nhóm màu đều hiện ra, kể cả khi thiếu trong bảng thứ tự", () => {
  // Bỏ sót một dòng trong bảng thứ tự là chuyện sẽ xảy ra. Hậu quả phải
  // là "màu ấy nằm sai chỗ", không phải "khách không lọc được màu ấy".
  const hien = nhomMauHienThi();
  expect(hien.length).toBe(Object.keys(NHOM_MAU).length);
  expect(new Set(hien).size).toBe(hien.length);
});
