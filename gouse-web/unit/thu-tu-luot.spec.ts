import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { ThuTuLuot } from "../apps/storefront/src/lib/thu-tu-luot";

/**
 * Kịch bản THẬT mà quy tắc này sinh ra để chặn.
 *
 * Trang vừa tải nên `GET /cart` chạy chậm. Khách bấm "Thêm vào giỏ". Lời
 * gọi thêm xong TRƯỚC. Rồi `GET /cart` cũ về sau.
 *
 * Không có quy tắc, phản hồi cũ ghi đè: món khách vừa thêm biến mất khỏi
 * màn hình dù máy chủ đã ghi nhận. Họ bấm thêm lần nữa — và giờ có hai
 * món trong giỏ.
 */
test("phản hồi CŨ về sau thì bị bỏ", () => {
  const t = new ThuTuLuot();

  const taiGio = t.batDau(); // lượt 1: tải giỏ, chậm
  const themMon = t.batDau(); // lượt 2: thêm món, nhanh

  // Lượt 2 về trước và ĐƯỢC nhận.
  expect(themMon(), "lượt mới nhất phải được nhận").toBe(true);

  // Lượt 1 về sau và phải BỊ BỎ.
  expect(
    taiGio(),
    "phản hồi cũ được nhận: nó sẽ ghi đè giỏ mới bằng dữ liệu cũ hơn",
  ).toBe(false);
});

test("lượt duy nhất luôn được nhận", () => {
  const t = new ThuTuLuot();
  const chi1 = t.batDau();
  expect(chi1()).toBe(true);
});

test("lượt mới nhất vẫn được nhận sau nhiều lượt chồng nhau", () => {
  const t = new ThuTuLuot();
  const a = t.batDau();
  const b = t.batDau();
  const c = t.batDau();

  expect(c(), "lượt cuối phải được nhận").toBe(true);
  expect(a()).toBe(false);
  expect(b()).toBe(false);
});

/**
 * Hỏi nhiều lần KHÔNG được đổi câu trả lời.
 *
 * `finally` và `catch` cùng hỏi một lượt; một hàm có tác dụng phụ ở đây sẽ
 * cho hai câu trả lời khác nhau trong cùng một lần xử lý.
 */
test("hỏi lại cho cùng câu trả lời", () => {
  const t = new ThuTuLuot();
  const cu = t.batDau();
  t.batDau();

  expect(cu()).toBe(false);
  expect(cu()).toBe(false);
});

/**
 * Hai bộ đếm ĐỘC LẬP.
 *
 * Mỗi provider giữ bộ đếm riêng; dùng chung một bộ đếm toàn cục thì giỏ
 * hàng của một tab sẽ hủy lượt của tab khác.
 */
test("hai bộ đếm không ảnh hưởng nhau", () => {
  const t1 = new ThuTuLuot();
  const t2 = new ThuTuLuot();

  const a = t1.batDau();
  t2.batDau();
  t2.batDau();

  expect(a(), "lượt của bộ đếm 1 không bị bộ đếm 2 làm cũ").toBe(true);
});

/**
 * Provider PHẢI dùng quy tắc, không chỉ có nó nằm cạnh.
 *
 * # Vì sao quét mã nguồn thay vì kiểm hành vi
 *
 * Kiểm hành vi cần render `ShopProvider` bằng React. Không làm được với bộ
 * chạy hiện tại: Playwright biến đổi JSX trong `.tsx` sang định dạng
 * component-testing của nó, nên `ShopProvider` trả về một object lạ chứ
 * không phải React element — thử với @testing-library/react thì hỏng ngay
 * ở lời gọi `render` đầu tiên.
 *
 * Bài quét này YẾU HƠN hẳn: nó chỉ thấy được đoạn mã có mặt, không thấy
 * được nó chạy đúng. Nhưng nó bắt được kiểu hỏng thực tế nhất — ai đó dọn
 * `run()` và bỏ mất hai dòng kiểm — và đó là khoảng trống đã lộ ra khi phá
 * để kiểm: gỡ quy tắc khỏi provider mà KHÔNG test nào đỏ.
 *
 * Muốn kiểm hành vi thật thì cần một bộ chạy render được React (vitest +
 * jsdom, hoặc Playwright component testing). Đó là quyết định về công cụ,
 * ghi ở backlog mục 2.24.
 */
test("ShopProvider dùng ThuTuLuot trong run()", () => {
  // Đường dẫn từ THƯ MỤC CHẠY, không dùng `import.meta.url`: nó đẩy tệp
  // sang chế độ ESM và làm hỏng phần import còn lại của bộ chạy.
  const nguon = readFileSync(
    "apps/storefront/src/lib/shop.tsx",
    "utf8",
  );

  expect(nguon, "phải nhập ThuTuLuot").toContain("ThuTuLuot");

  // Hai chỗ kiểm: một cho đường thành công, một cho đường lỗi.
  const soLanKiem = (nguon.match(/if \(!conMoiNhat\(\)\) return;/g) ?? []).length;
  expect(
    soLanKiem,
    "run() phải bỏ kết quả của lượt cũ ở CẢ đường thành công lẫn đường lỗi",
  ).toBe(2);
});
