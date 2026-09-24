import { expect, test } from "@playwright/test";

import { watchApi } from "./loi-mang";

/**
 * Bộ lọc màu của danh mục.
 *
 * # Vì sao qua trình duyệt thật
 *
 * Logic lọc đã có test ở backend — và có cả một bài bắt HAI KHO phải trả
 * lời giống nhau (`TestHaiKhoLocBienTheGiongNhau`). Lặp lại chúng ở đây là
 * trả giá đắt cho cùng một câu trả lời.
 *
 * Ba thứ chỉ chỗ này thấy được:
 *
 *	tham số có RỜI trình duyệt không   mảng `color` phải ghép bằng dấu
 *	                                   phẩy trước khi lên dây. Ghép sai
 *	                                   thì client biên dịch được, máy chủ
 *	                                   bỏ qua tham số, và danh mục trả về
 *	                                   ĐỦ — trông hệt như "không có lỗi"
 *	Suspense quanh useSearchParams     thiếu nó thì `next build` hỏng,
 *	                                   còn `next dev` vẫn chạy ngon
 *	trạng thái sống qua tải lại        bộ lọc ở URL chứ không ở useState
 *
 * # Không giả định danh mục có màu gì
 *
 * Dữ liệu mẫu đổi theo từng lần nạp. Bài test đọc danh sách TRƯỚC và SAU
 * khi lọc rồi so hai tập với nhau, thay vì chờ một tên sản phẩm cụ thể.
 */

type Page = import("@playwright/test").Page;

const LA_DANH_MUC = (u: string) => u.includes("/api/v1/products") && !u.includes("ids=");

/**
 * Chạy `hanhDong` rồi chờ ĐÚNG lượt gọi danh mục mà nó gây ra.
 *
 * # Vì sao không dùng `networkidle`
 *
 * Bản đầu của bài này dùng `waitForLoadState("networkidle")` sau mỗi cú
 * bấm, và nó trả về NGAY — lượt gọi mới chưa kịp bắt đầu. Mỗi lần đọc
 * danh sách đều là dữ liệu CŨ, nên vòng lặp không bao giờ thấy danh mục
 * thu hẹp, và bài test tự `skip` chính mình.
 *
 * Một bài bị bỏ qua trông hệt như một bài đã chạy: bộ chạy in màu xanh,
 * tổng số tăng lên, và không ai đọc chữ "skipped".
 *
 * Chờ đúng response thì vừa xác định vừa cho ta đọc luôn tham số ĐÃ RỜI
 * trình duyệt — thứ bài này tồn tại để kiểm.
 */
async function bamVaChoDanhMuc(page: Page, hanhDong: () => Promise<void>) {
  const [res] = await Promise.all([
    page.waitForResponse((r) => LA_DANH_MUC(r.url()) && r.request().method() === "GET"),
    hanhDong(),
  ]);
  const body = (await res.json()) as { data?: unknown[] };
  const soMong = body.data?.length ?? 0;

  // Chờ React vẽ xong đúng số thẻ ấy — `toHaveCount` tự thử lại.
  await expect(page.locator(".card__name")).toHaveCount(soMong);
  return {
    ten: await page.locator(".card__name").allTextContents(),
    thamSoMau: new URL(res.url()).searchParams.get("color"),
  };
}

async function moDanhMuc(page: Page, duong: string) {
  return bamVaChoDanhMuc(page, async () => {
    await page.goto(duong);
    await expect(page.getByRole("heading", { name: "Sản phẩm" })).toBeVisible();
  });
}

test("lọc màu THU HẸP danh mục, và trạng thái sống qua tải lại", async ({ page }) => {
  const loi = watchApi(page);

  const { ten: tatCa } = await moDanhMuc(page, "/");

  // Danh mục rỗng là trạng thái duy nhất bỏ qua được: không có hàng thì
  // không bộ lọc nào chứng minh được gì. Mọi trường hợp khác phải ĐỎ.
  test.skip(tatCa.length === 0, "danh mục rỗng — không có gì để lọc");

  // Tìm một màu THỰC SỰ thu hẹp được: bấm lần lượt cho tới khi số sản
  // phẩm giảm. Không hardcode "Đen" vì dữ liệu mẫu có thể không có màu ấy.
  const nut = page.locator("button.mau");
  const soMau = await nut.count();
  expect(soMau, "bộ lọc màu phải hiện ra").toBeGreaterThan(0);

  // Tìm một màu ĐỔI được kết quả. Không hardcode "Đen": dữ liệu mẫu có
  // thể không có màu ấy.
  //
  // Điều kiện là ĐỔI, không phải THU HẸP. Nếu cả danh mục chỉ có một nhóm
  // màu thì không màu nào thu hẹp được — nhưng mọi màu KHÁC phải trả về
  // rỗng, và đó vẫn là bằng chứng bộ lọc đang chạy.
  let daLoc: string[] | null = null;
  let tenMau = "";
  let thamSo: string | null = null;
  for (let i = 0; i < soMau; i++) {
    const n = nut.nth(i);
    tenMau = (await n.textContent())?.trim() ?? "";
    const ra = await bamVaChoDanhMuc(page, () => n.click());
    if (ra.ten.join("|") !== tatCa.join("|")) {
      daLoc = ra.ten;
      thamSo = ra.thamSoMau;
      break;
    }
    await bamVaChoDanhMuc(page, () => n.click()); // tắt đi, thử màu kế tiếp
  }

  // KHÔNG `test.skip` ở đây.
  //
  // Bản đầu viết `test.skip(daLoc === null, …)`, và khi tôi cố tình phá
  // `listProducts` để nó không gửi mảng lên dây, bài này BỎ QUA chính nó
  // thay vì đỏ — bộ chạy in "2 passed, 1 skipped" và trông như đã xanh.
  //
  // Mười bốn màu mà không màu nào đổi được kết quả nghĩa là bộ lọc không
  // tới được máy chủ. Đó là chính lỗi bài này sinh ra để bắt.
  expect(
    daLoc,
    `bấm đủ ${soMau} màu mà danh mục không đổi — bộ lọc không tới máy chủ`,
  ).not.toBeNull();

  // Tham số PHẢI rời trình duyệt. Đây là chỗ hỏng im lặng: mảng không
  // ghép thành chuỗi thì máy chủ bỏ qua và danh mục trả về ĐỦ — trông hệt
  // như không có lỗi.
  expect(thamSo, "tham số color phải nằm trong request thật").toBeTruthy();
  expect(new URL(page.url()).searchParams.get("color")).toBe(thamSo);

  // Kết quả lọc phải là TẬP CON, không phải một danh sách khác: lọc chỉ
  // được BỚT đi, không được mang về thứ danh mục đầy đủ không có.
  for (const t of daLoc!) expect(tatCa).toContain(t);

  await expect(
    page.getByRole("button", { name: tenMau }),
  ).toHaveAttribute("aria-pressed", "true");

  // Tải lại: bộ lọc ở URL nên phải sống sót — cả kết quả lẫn nút đang bật.
  const sauTaiLai = await bamVaChoDanhMuc(page, () => page.reload().then(() => {}));
  expect(sauTaiLai.ten).toEqual(daLoc);
  expect(sauTaiLai.thamSoMau).toBe(thamSo);
  await expect(
    page.getByRole("button", { name: tenMau }),
  ).toHaveAttribute("aria-pressed", "true");

  expect(loi, "không request nào được hỏng").toEqual([]);
});

test("màu không có hàng nói ĐÚNG lý do, không nói cửa hàng trống", async ({ page }) => {
  // "Chưa có sản phẩm nào được đăng bán" trong lúc khách vừa lọc màu là
  // một câu SAI: nó khiến họ nghĩ cửa hàng trống chứ không nghĩ tới việc
  // bỏ bộ lọc.
  await page.goto("/?color=PURPLE,ORANGE,YELLOW,SILVER");
  await expect(page.getByRole("heading", { name: "Sản phẩm" })).toBeVisible();

  const rong = page.getByText("Không có sản phẩm nào màu");
  const coHang = page.locator(".card__name").first();

  // Một trong hai phải đúng: hoặc có hàng, hoặc thông báo nói rõ vì sao.
  await expect(rong.or(coHang)).toBeVisible({ timeout: 15_000 });
  if (await rong.isVisible()) {
    await expect(page.getByRole("button", { name: "Bỏ lọc màu" })).toBeVisible();
    await expect(page.getByText("Chưa có sản phẩm nào được đăng bán")).toHaveCount(0);
  }
});

test("nhóm màu LẠ trong URL không lọc gì và không làm hỏng trang", async ({ page }) => {
  // `?color=CAM_VANG` là thứ ai cũng gõ được vào thanh địa chỉ. Gửi thẳng
  // lên máy chủ thì nó trả rỗng và khách thấy cửa hàng trống.
  const { ten: tatCa } = await moDanhMuc(page, "/");
  const rac = await moDanhMuc(page, "/?color=CAM_VANG");

  expect(rac.ten).toEqual(tatCa);

  // …và tham số rác KHÔNG được gửi lên máy chủ: gửi thẳng thì nó trả rỗng
  // và khách thấy cửa hàng trống.
  expect(rac.thamSoMau).toBeNull();
});
