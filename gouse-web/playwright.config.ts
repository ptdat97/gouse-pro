import { defineConfig, devices } from "@playwright/test";

/**
 * Cấu hình kiểm thử đầu-cuối cho giao diện.
 *
 * # Phạm vi: chỉ những gì CHỈ trình duyệt mới thấy
 *
 * Logic nghiệp vụ đã có test ở backend, và test ở đó nhanh hơn hai bậc
 * độ lớn. Lặp lại chúng qua trình duyệt là trả giá đắt cho cùng một câu
 * trả lời.
 *
 * Những thứ CHỈ chỗ này bắt được:
 *
 *   - CORS. Origin thiếu trong danh sách trắng làm mọi lời gọi hỏng, và
 *     lỗi hiện ở console trình duyệt trong khi log máy chủ sạch trơn. Đã
 *     dính ba lần: cổng 3001, header X-Guest-Phone, cổng 3002.
 *   - Cookie phiên đi qua nhiều trang.
 *   - Trang lỗi lúc dựng sẵn (prerender) — `useSearchParams` không bọc
 *     Suspense chỉ nổ khi build, không nổ khi gõ code.
 *   - Kiểu dữ liệu khớp lúc biên dịch nhưng THIẾU lúc chạy: trường không
 *     `required` trong đặc tả mà máy chủ không bao giờ trả.
 *
 * # Vì sao không tự khởi động máy chủ
 *
 * Bộ test này chạy trên stack ĐANG CHẠY: API cổng 8080, cửa hàng 3001,
 * trung tâm người bán 3002. Tự dựng cả ba sẽ cần database riêng, dữ liệu
 * mẫu riêng và khoảng một phút mỗi lần chạy.
 *
 * Đổi lại, test PHẢI tự tạo dữ liệu của mình qua API và không được giả
 * định trạng thái sẵn có — trừ danh mục sản phẩm.
 */
export default defineConfig({
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",

  use: {
    baseURL: process.env.STOREFRONT_URL ?? "http://localhost:3001",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    locale: "vi-VN",
  },

  projects: [
    /**
     * Test ĐƠN VỊ cho logic thuần, không mở trình duyệt.
     *
     * Dùng chung bộ chạy với test đầu-cuối thay vì thêm một phụ thuộc
     * nữa: Playwright chạy TypeScript sẵn, và một bộ chạy ít hơn là một
     * cấu hình ít hơn để lệch nhau.
     *
     * Ranh giới rõ ràng: `unit/` KHÔNG được chạm `page`, và không cần
     * máy chủ nào đang chạy.
     */
    {
      name: "unit",
      testDir: "./unit",
    },
    {
      name: "chromium",
      testDir: "./e2e",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
