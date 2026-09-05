import { expect, test } from "@playwright/test";

import { countdown, msConLai } from "../apps/storefront/src/lib/format";

function sauNay(giay: number): string {
  return new Date(Date.now() + giay * 1000).toISOString();
}

test("đếm ngược hiện phút:giây", () => {
  expect(countdown(sauNay(754))).toMatch(/^12:3[0-4]$/);
});

test("hết hạn hiện 00:00", () => {
  expect(countdown(sauNay(-60))).toBe("00:00");
});

/**
 * "—" và "00:00" nói HAI chuyện khác nhau.
 *
 * `—` là KHÔNG BIẾT; `00:00` là ĐÃ HẾT HẠN. Gộp chúng lại tạo ra một trang
 * tự mâu thuẫn: `expires_at` hỏng làm phép tính của trang thành NaN, mà
 * `NaN <= 0` là false — nên trang coi phiên CHƯA hết hạn và cho bấm tiếp,
 * trong khi đồng hồ hiện "00:00".
 *
 * Khách đọc "Hàng được giữ cho bạn trong 00:00" và không biết tin cái nào.
 */
test("mốc thời gian hỏng là KHÔNG BIẾT, không phải đã hết hạn", () => {
  for (const xau of ["", "khong-phai-ngay", "2026-13-45T99:99:99Z"]) {
    expect(countdown(xau)).toBe("—");
    expect(msConLai(xau)).toBeNull();
  }
  expect(countdown(undefined)).toBe("—");
  expect(msConLai(undefined)).toBeNull();
});

/**
 * `null` phải KHÁC 0.
 *
 * Trả 0 cho giá trị không đọc được sẽ ép mọi bên gọi coi phiên là đã hết
 * hạn — và trang thanh toán khi đó KHÓA hết nút, chặn khách vì một mốc
 * thời gian hỏng chứ không phải vì phiên thật sự hết.
 */
test("không biết KHÁC không còn thời gian", () => {
  expect(msConLai(undefined)).not.toBe(0);
  const hetHan = msConLai(sauNay(-10));
  expect(hetHan).not.toBeNull();
  expect(hetHan!).toBeLessThanOrEqual(0);
});

test("còn thời gian thì trả số dương", () => {
  const con = msConLai(sauNay(300));
  expect(con).not.toBeNull();
  expect(con!).toBeGreaterThan(290_000);
});
