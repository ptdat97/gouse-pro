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

/**
 * laySkuBatKy lấy một mã SKU CÓ THẬT từ danh mục.
 *
 * Không hardcode mã: dữ liệu mẫu sinh ULID mới mỗi lần nạp lại, nên mã
 * viết cứng sẽ hỏng ở lần seed tiếp theo — và hỏng theo kiểu khó hiểu
 * ("không tìm thấy SKU") chứ không phải kiểu nói rõ nguyên nhân.
 */
async function laySkuBatKy(
  page: import("@playwright/test").Page,
): Promise<string> {
  const api = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

  const ds = await page.request.get(`${api}/api/v1/products?limit=1`);
  const products = (await ds.json()).data ?? [];
  if (products.length === 0) throw new Error("danh mục trống");

  const ct = await page.request.get(`${api}/api/v1/products/${products[0].id}`);
  const body = await ct.json();
  for (const v of body.variants ?? []) {
    for (const sku of v.skus ?? []) {
      if (sku.id) return sku.id;
    }
  }
  throw new Error("sản phẩm không có SKU nào");
}

/**
 * datMotDonMoi đặt một đơn qua API để nhà bán CÓ VIỆC mà làm.
 *
 * # Vì sao test tự dựng dữ liệu thay vì dùng dữ liệu có sẵn
 *
 * Bài bàn giao vận chuyển TIÊU THỤ chính thứ nó cần: bàn giao xong thì
 * đơn rời hàng chờ. Dựa vào dữ liệu mẫu nghĩa là xanh đúng một lần, rồi
 * những lần sau lặng lẽ bị bỏ qua — và một bài test bị bỏ qua trông y hệt
 * một bài test đã chạy.
 *
 * Đặt qua API chứ không qua giao diện cửa hàng: bài này kiểm TRUNG TÂM
 * NGƯỜI BÁN, và dựng dữ liệu qua giao diện khác sẽ làm nó đỏ vì lý do
 * chẳng liên quan.
 */
async function datMotDonMoi(page: import("@playwright/test").Page) {
  const api = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
  const khoa = () => ({ "Idempotency-Key": `req_${Date.now()}${Math.random()}` });

  const ds = await page.request.get(`${api}/api/v1/products?limit=1`);
  const productID = ((await ds.json()).data ?? [])[0]?.id;
  if (!productID) throw new Error("danh mục trống");

  const offers = await page.request.get(
    `${api}/api/v1/products/${productID}/offers`,
  );
  const offer = ((await offers.json()).data ?? []).find(
    (o: { is_sellable?: boolean }) => o.is_sellable,
  );
  if (!offer) throw new Error("không có offer nào bán được");

  const gio = await page.request.post(`${api}/api/v1/cart/items`, {
    headers: khoa(),
    data: { offer_id: offer.id, quantity: 1 },
  });
  const cartID = (await gio.json()).cart?.id;

  const phien = await page.request.post(`${api}/api/v1/checkout`, {
    headers: khoa(),
    data: {
      cart_id: cartID,
      guest_email: "nhaban-e2e@example.com",
      guest_phone: "0900999888",
    },
  });
  const checkoutID = (await phien.json()).checkout?.id;

  await page.request.patch(
    `${api}/api/v1/checkout/${checkoutID}/shipping-address`,
    {
      headers: khoa(),
      data: {
        recipient_name: "Người Nhận E2E",
        phone: "0900999888",
        street_address: "1 Đường Thử",
        ward: "Phường 1",
        district: "Quận 1",
        province: "TP.HCM",
        country_code: "VN",
      },
    },
  );
  await page.request.patch(
    `${api}/api/v1/checkout/${checkoutID}/shipping-method`,
    { headers: khoa(), data: { shipping_method: "STANDARD" } },
  );
  await page.request.post(`${api}/api/v1/checkout/${checkoutID}/complete`, {
    headers: khoa(),
    data: { payment_method: "COD" },
  });
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

  /**
   * Bàn giao vận chuyển — bước tiền RA khỏi kho.
   *
   * Đây là hành động không lùi được: sau khi bàn giao, đơn không hủy được
   * nữa (quy tắc 8 của fulfillment — từ PACKED trở đi đã tốn công và vật
   * tư). Nên nút này có hai lớp chặn, và bài test kiểm cả hai.
   */
  test("bàn giao vận chuyển ghi được mã vận đơn", async ({ page }) => {
    const loi = watchApi(page);
    await dangNhap(page);

    // Tự tạo việc cho mình: xem chú thích của datMotDonMoi.
    await datMotDonMoi(page);

    const phieu = page.locator("section.panel").filter({
      has: page.getByRole("button", { name: "Đã bàn giao vận chuyển" }),
    });

    // Đơn thực hiện được tạo BẤT ĐỒNG BỘ.
    //
    // `checkout.completed` vào outbox, worker đọc theo nhịp rồi mới tách
    // đơn theo nguồn hàng. Nhìn ngay sau khi đặt là nhìn quá sớm — và đó
    // là hành vi ĐÚNG của kiến trúc event, không phải chỗ cần sửa.
    //
    // KHÔNG dùng test.skip khi chưa thấy: một bài test bị bỏ qua trông y
    // hệt một bài test đã chạy, và đó là cách hỏng tệ nhất của bộ test.
    await expect
      .poll(
        async () => {
          await page.goto("/");
          return phieu.count();
        },
        {
          message: "worker chưa tách đơn thực hiện sau khi đặt hàng",
          timeout: 30_000,
          intervals: [1000, 2000, 3000],
        },
      )
      .toBeGreaterThan(0);

    // Chọn đơn BÀN GIAO ĐƯỢC, không phải đơn đầu tiên.
    //
    // Dữ liệu phát triển còn những đơn cũ tách trước khi có địa chỉ giao;
    // nút của chúng bị KHÓA có chủ ý — giao nhầm địa chỉ đoán ra tốn hơn
    // nhiều so với chờ hỏi lại. Lấy `first()` sẽ trúng một đơn như vậy và
    // bài test tự bỏ qua chính mình, ngẫu nhiên theo thứ tự dữ liệu.
    //
    // Đồng thời KHẲNG ĐỊNH luôn quy tắc đó: đơn nào thiếu địa chỉ thì nút
    // phải khóa.
    let dau = phieu.first();
    let nut = dau.getByRole("button", { name: "Đã bàn giao vận chuyển" });
    let timThay = false;

    const soPhieu = await phieu.count();
    for (let i = 0; i < soPhieu; i++) {
      const p = phieu.nth(i);
      const b = p.getByRole("button", { name: "Đã bàn giao vận chuyển" });
      if (await b.isEnabled()) {
        dau = p;
        nut = b;
        timThay = true;
        break;
      }
      // Nút khóa thì PHẢI có lời nhắc nói vì sao.
      await expect(p.getByText(/địa chỉ/i)).toBeVisible();
    }
    expect(timThay, "không có đơn nào bàn giao được").toBe(true);

    const maVanDon = `GHN${Date.now().toString().slice(-8)}`;
    await dau.getByLabel("Đơn vị vận chuyển").fill("GHN");
    await dau.getByLabel("Mã vận đơn").fill(maVanDon);
    await nut.click();

    // Đơn rời khỏi hàng chờ và xuất hiện ở mục "Đã bàn giao", kèm mã.
    await expect(page.getByRole("heading", { name: "Đã bàn giao" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(page.getByText(maVanDon)).toBeVisible();

    expect(loi.map((l) => l.mo_ta)).toEqual([]);
  });

  /**
   * Đăng bán một sản phẩm mới.
   *
   * `initial_inventory` KHÔNG phải tùy chọn trong thực tế: offer không có
   * hàng thì hết hàng ngay từ giây đầu, và không có đường nào nhập sau —
   * kiểm kê chỉ SỬA bản ghi đã có.
   */
  test("đăng bán sản phẩm mới kèm tồn kho ban đầu", async ({ page }) => {
    const loi = watchApi(page);
    await dangNhap(page);
    await page.goto("/offers");

    const truoc = await page.locator("section.panel").count();

    await page.getByRole("button", { name: "Đăng bán sản phẩm" }).click();

    // Mã SKU lấy từ chính danh mục: offer phải trỏ tới hàng CÓ THẬT.
    const skuID = await laySkuBatKy(page);
    await page.getByLabel("Mã SKU").fill(skuID);
    await page.getByLabel("Giá bán (đ)").fill("259000");
    await page.getByLabel("Số lượng có sẵn").fill("7");
    await page.getByRole("button", { name: "Đăng bán", exact: true }).click();

    // Danh sách dài thêm, HOẶC báo lỗi rõ ràng nếu SKU đã có offer.
    //
    // Một seller chỉ được một offer ACTIVE cho mỗi SKU — quy tắc domain.
    // Chấp nhận cả hai kết cục vì dữ liệu mẫu có thể đã dùng hết SKU, và
    // một test đòi hỏi trạng thái ban đầu chính xác là test sẽ chập chờn.
    await expect
      .poll(
        async () => {
          const sau = await page.locator("section.panel").count();
          const coLoi = await page.getByText(/đã có offer|không đăng bán được/i).count();
          return sau > truoc || coLoi > 0;
        },
        { message: "không thấy offer mới cũng không thấy lỗi", timeout: 10_000 },
      )
      .toBe(true);

    expect(loi.map((l) => l.mo_ta)).toEqual([]);
  });
});
