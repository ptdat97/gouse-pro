import { expect, test } from "@playwright/test";

import { countItems } from "../apps/storefront/src/lib/shop";

function gio(nhom: { items?: unknown[] }[]) {
  return { cart: { groups: nhom } } as never;
}

/**
 * Con số trên biểu tượng giỏ là ĐẾM DÒNG, không cộng số lượng.
 *
 * Đây là quyết định sản phẩm, không phải chi tiết cài đặt: "3" nghĩa là ba
 * món khác nhau. Đổi sang cộng số lượng thì khách mua ba cái cùng một áo
 * cũng thấy "3", và con số mất hết ý nghĩa phân biệt.
 *
 * Quyết định đó nằm trong một chú thích và không có gì ghim — một lần
 * "sửa cho đúng hơn" là đủ để nó biến mất.
 */
test("đếm DÒNG, không cộng số lượng", () => {
  const motDongBaCai = gio([{ items: [{ quantity: 3 }] }]);
  expect(countItems(motDongBaCai)).toBe(1);

  const baDong = gio([{ items: [{}, {}, {}] }]);
  expect(countItems(baDong)).toBe(3);
});

test("cộng qua NHIỀU nhóm nhà bán", () => {
  // Đơn trộn hàng nhiều nhà bán là chuyện bình thường ở chợ.
  expect(countItems(gio([{ items: [{}, {}] }, { items: [{}] }]))).toBe(3);
});

/**
 * Giỏ rỗng hoặc chưa tải xong KHÔNG được làm nổ trang.
 *
 * Badge nằm ở thanh điều hướng, hiện trên MỌI trang — một lỗi ném ở đây
 * làm trắng toàn bộ ứng dụng, không riêng trang giỏ hàng.
 */
test("giỏ thiếu hoặc rỗng thì trả 0, không nổ", () => {
  expect(countItems(null)).toBe(0);
  expect(countItems({} as never)).toBe(0);
  expect(countItems({ cart: {} } as never)).toBe(0);
  expect(countItems(gio([]))).toBe(0);
  expect(countItems(gio([{}]))).toBe(0);
});
