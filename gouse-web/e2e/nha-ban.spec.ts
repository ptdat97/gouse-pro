import { expect, test } from "@playwright/test";

import { watchApi } from "./loi-mang";

const SELLER_URL = process.env.SELLER_URL ?? "http://localhost:3002";
const EMAIL = process.env.SELLER_EMAIL ?? "nhaban@example.com";
const MAT_KHAU = process.env.SELLER_PASSWORD ?? "mat-khau-du-dai-123";

/**
 * Trung tâm người bán chạy ở ORIGIN KHÁC (cổng 3002).
 *
 * Đó là toàn bộ lý do các bài test này tồn tại tách khỏi cửa hàng: mỗi
 * origin mới là một dòng phải thêm vào danh sách trắng CORS, và quên nó
 * làm hỏng TOÀN BỘ ứng dụng đó trong khi hai ứng dụng kia vẫn chạy bình
 * thường. Log máy chủ không ghi gì.
 */

/**
 * dangNhap đăng nhập rồi CHỜ tới khi phiên thực sự sẵn sàng.
 *
 * Chờ là bắt buộc, không phải cho chắc: `click` chỉ khởi động lời gọi
 * đăng nhập. Điều hướng ngay sau đó sẽ hủy lời gọi giữa chừng, cookie làm
 * mới chưa kịp đặt, và trang mới hiện lại form đăng nhập — trông hệt như
 * lỗi khôi phục phiên trong khi ứng dụng không hề sai.
 *
 * Mốc chờ là thanh điều hướng: nó chỉ dựng khi đã có hồ sơ người dùng.
 */
async function dangNhap(page: import("@playwright/test").Page) {
  await page.goto("/");
  const email = page.getByLabel(/Email/i);
  await expect(email).toBeVisible({ timeout: 15_000 });
  await email.fill(EMAIL);
  await page.getByLabel(/Mật khẩu/i).fill(MAT_KHAU);
  await page.getByRole("button", { name: /Đăng nhập/i }).click();

  await expect(
    page.getByRole("link", { name: "Hàng đang bán" }),
  ).toBeVisible({ timeout: 15_000 });
}

test.describe("Trung tâm người bán", () => {
  test.use({ baseURL: SELLER_URL });

  test("đăng nhập rồi thấy hàng đang bán", async ({ page }) => {
    const loi = watchApi(page);

    await dangNhap(page);

    await page.getByRole("link", { name: "Hàng đang bán" }).click();
    await expect(
      page.getByRole("heading", { name: "Hàng đang bán" }),
    ).toBeVisible();

    // Phải có ít nhất một offer, nếu không thì không có gì để quản lý.
    await expect(page.getByText(/^SKU sku_/).first()).toBeVisible({
      timeout: 15_000,
    });

    expect(loi.map((l) => l.mo_ta)).toEqual([]);
  });

  /**
   * Kiểm kê nhận con số ĐÃ ĐẾM và trả về con số THẬT trong kho.
   *
   * Hai con số đó lệch nhau khi có đơn chen vào giữa lúc ghi, nên giao
   * diện phải hiện cái máy chủ trả về chứ không phải cái vừa gõ.
   */
  test("kiểm kê ghi được và hiện số máy chủ trả về", async ({ page }) => {
    const loi = watchApi(page);

    await dangNhap(page);

    // Điều hướng CỨNG, không phải bấm link: access token nằm trong bộ nhớ
    // nên tải lại trang là mất, và ứng dụng phải khôi phục phiên bằng
    // cookie làm mới. Đường này chỉ chạy khi người dùng gõ thẳng URL hoặc
    // bấm F5 — tức là thường xuyên.
    await page.goto("/offers");
    await expect(
      page.getByRole("heading", { name: "Hàng đang bán" }),
    ).toBeVisible({ timeout: 15_000 });

    const soLuong = page.getByLabel("Số lượng đã đếm").first();
    await expect(soLuong).toBeVisible({ timeout: 15_000 });
    await soLuong.fill("77");
    await page.getByLabel("Lý do").first().fill("Kiem ke tu dong E2E");
    await page.getByRole("button", { name: "Cập nhật kho" }).first().click();

    await expect(page.getByText(/Tồn kho hiện tại: 77/)).toBeVisible({
      timeout: 15_000,
    });

    expect(loi.map((l) => l.mo_ta)).toEqual([]);
  });
});
