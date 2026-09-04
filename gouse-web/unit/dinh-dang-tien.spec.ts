import { expect, test } from "@playwright/test";

import { money as moneyStorefront } from "../apps/storefront/src/lib/format";
import { money as moneyAdmin } from "../apps/admin/src/lib/format";
import { money as moneySeller } from "../apps/seller/src/lib/format";

/**
 * Ba app có ba bản `money()` riêng. Bài này chạy CẢ BA qua cùng bộ dữ liệu:
 * sửa một bản mà quên hai bản kia là lỗi rất dễ mắc, và nó không lộ ra ở
 * bất kỳ chỗ nào khác.
 */
const banSao = [
  { ten: "storefront", money: moneyStorefront },
  { ten: "admin", money: moneyAdmin },
  { ten: "seller", money: moneySeller },
];

for (const { ten, money } of banSao) {
  test(`${ten}: chữ thường "vnd" KHÔNG được chia cho 100`, () => {
    /**
     * Lỗi tệ nhất một trang bán hàng có thể mắc.
     *
     * So sánh phân biệt hoa thường làm `"vnd"` không khớp `"VND"`, nên số
     * tiền bị chia 100: 250.000 ₫ hiện thành 2.500 ₫ — vẫn kèm ký hiệu ₫
     * nên trông hoàn toàn hợp lệ, và không có gì báo.
     */
    const hoa = money({ amount: 250_000, currency: "VND" });
    const thuong = money({ amount: 250_000, currency: "vnd" });
    expect(thuong).toBe(hoa);
    expect(thuong).toContain("250");
  });

  test(`${ten}: mã tiền tệ hỏng KHÔNG được ném lỗi`, () => {
    /**
     * `Intl.NumberFormat` ném RangeError với mã không hợp lệ. Một lỗi ném
     * lúc render làm TRẮNG cả trang thay vì hiện một ô sai.
     */
    for (const currency of ["", "ABCD", "1 2", "€$¥"]) {
      expect(() => money({ amount: 250_000, currency })).not.toThrow();
    }
  });

  test(`${ten}: đơn vị phụ vẫn đúng cho tiền tệ hai số lẻ`, () => {
    // 1234 cent = 12,34 USD. Không được vì bản sửa mà mất phép chia này.
    expect(money({ amount: 1234, currency: "USD" })).toContain("12");
    expect(money({ amount: 1234, currency: "usd" })).toBe(
      money({ amount: 1234, currency: "USD" }),
    );
  });

  test(`${ten}: thiếu số tiền thì hiện dấu gạch`, () => {
    expect(money(null)).toBe("—");
    expect(money(undefined)).toBe("—");
  });
}
