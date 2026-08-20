import type { Page } from "@playwright/test";

const API = "localhost:8080";

/**
 * Hai đường dò phiên trả 401 cho khách VÃNG LAI — đúng thiết kế, không
 * phải lỗi.
 *
 * Giao diện giữ access token trong bộ nhớ, nên tải lại trang là mất. Nó
 * gọi thử `/me`, nhận 401, thử làm mới bằng cookie, lại 401, rồi coi người
 * dùng là khách vãng lai và đi tiếp. Khách vãng lai MUA ĐƯỢC HÀNG nên đây
 * là trạng thái bình thường, không phải hỏng.
 */
const DO_PHIEN = ["/api/v1/me", "/api/v1/auth/refresh", "/api/v1/admin/me"];

export type Loi = { mo_ta: string };

/**
 * watchApi theo dõi mọi lời gọi tới API và ghi lại thứ THẬT SỰ hỏng.
 *
 * # Vì sao không bắt hết console
 *
 * Console lẫn nhiều nhiễu không liên quan tới sức khỏe hệ thống: ảnh mẫu
 * trỏ tới cdn.example.com không phân giải được, cảnh báo của framework,
 * v.v. Bắt hết thì test đỏ vì lý do sai, và test đỏ vì lý do sai sẽ bị bỏ
 * qua — rồi lần đỏ THẬT cũng bị bỏ qua theo.
 *
 * # Hai thứ được bắt, và vì sao đúng là hai thứ này
 *
 *  1. **Lời gọi hỏng ở tầng mạng.** Đây là hình dạng của lỗi CORS trong
 *     trình duyệt: request không bao giờ tới tay JavaScript, log máy chủ
 *     hoàn toàn sạch, và triệu chứng duy nhất là trang trống. Dự án đã
 *     dính ba lần — cổng 3001, header X-Guest-Phone, cổng 3002.
 *  2. **5xx.** Máy chủ tự nhận là mình hỏng.
 *
 * 4xx KHÔNG bị bắt: chúng thường là câu trả lời hợp lệ cho một câu hỏi
 * hợp lệ (chưa đăng nhập, không tìm thấy). Test nào cần khẳng định về 4xx
 * thì khẳng định thẳng vào lời gọi đó.
 */
export function watchApi(page: Page): Loi[] {
  const loi: Loi[] = [];

  page.on("requestfailed", (r) => {
    if (!r.url().includes(API)) return;
    loi.push({
      mo_ta:
        `lời gọi API hỏng ở tầng mạng (dấu hiệu điển hình của CORS): ` +
        `${r.method()} ${r.url()} — ${r.failure()?.errorText}`,
    });
  });

  page.on("response", (r) => {
    if (!r.url().includes(API)) return;
    if (r.status() >= 500) {
      loi.push({ mo_ta: `${r.status()} ${r.request().method()} ${r.url()}` });
    }
  });

  return loi;
}

/** guestOk lọc bỏ 401 của đường dò phiên. */
export function laDoPhien(url: string): boolean {
  return DO_PHIEN.some((p) => url.includes(p));
}
