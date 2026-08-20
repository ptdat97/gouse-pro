import { expect, test } from "@playwright/test";

import { watchApi } from "./loi-mang";

/**
 * Đường mua hàng của khách VÃNG LAI — đường ra tiền.
 *
 * Chạy qua trình duyệt thật vì đây là nơi DUY NHẤT thấy được ba thứ:
 * CORS, cookie phiên đi qua nhiều trang, và trường dữ liệu mà đặc tả cho
 * phép vắng mặt nhưng giao diện lại CẦN có.
 *
 * Thứ ba là lý do bài test này tồn tại. Nó bắt được ngay lần chạy đầu
 * tiên: nút "Thêm vào giỏ" khóa vĩnh viễn vì trang đọc `availability` —
 * một trường đặc tả có khai nhưng endpoint không bao giờ trả. Trường không
 * `required` nên TypeScript cho qua, backend xanh, log máy chủ sạch, và
 * cửa hàng không bán được gì.
 */
test("khách vãng lai mua được: chọn nhà bán → thêm giỏ → giỏ có hàng", async ({
  page,
}) => {
  const loi = watchApi(page);

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Sản phẩm" })).toBeVisible();

  // Vào sản phẩm ĐẦU TIÊN có thật thay vì hardcode mã: dữ liệu mẫu sinh
  // ULID mới mỗi lần nạp, nên mã cứng sẽ mục sau lần nạp kế tiếp.
  const card = page.locator("a.card__link").first();
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();

  await expect(page.getByRole("heading", { name: "Chọn nhà bán" })).toBeVisible();

  const offer = page.locator('input[name="offer"]').first();
  await expect(offer).toBeVisible();
  await offer.check();

  // Điểm mấu chốt của cả bài test.
  const nut = page.getByRole("button", { name: "Thêm vào giỏ" });
  await expect(
    nut,
    "nút thêm giỏ bị khóa — cửa hàng không bán được gì",
  ).toBeEnabled();
  await nut.click();

  await page.goto("/cart");
  await expect(page.getByRole("heading", { name: "Giỏ hàng" })).toBeVisible();
  await expect(page.getByText("Giỏ hàng đang trống")).toHaveCount(0);

  // Giỏ phải hiện TÊN nhà bán: khách cần biết đang mua của ai, và đó là
  // dữ liệu module cart trả sẵn.
  await expect(page.getByText(/^Bán bởi /)).toBeVisible();

  expect(loi.map((l) => l.mo_ta)).toEqual([]);
});

/**
 * Trang tra cứu đơn dùng header `X-Guest-Phone`.
 *
 * Header tùy chỉnh làm trình duyệt gửi preflight. Thiếu nó trong
 * `Access-Control-Allow-Headers` thì RIÊNG trang này hỏng trong khi mọi
 * trang khác vẫn chạy — đã xảy ra một lần, và không log máy chủ nào ghi.
 */
test("trang tra cứu đơn gọi được API kèm header tùy chỉnh", async ({ page }) => {
  const loi = watchApi(page);

  await page.goto("/orders");
  await expect(page.getByRole("heading", { name: /Đơn hàng|Tra cứu/ })).toBeVisible();

  expect(loi.map((l) => l.mo_ta)).toEqual([]);
});
