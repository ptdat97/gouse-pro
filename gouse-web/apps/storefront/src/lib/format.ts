/**
 * Định dạng để HIỂN THỊ.
 *
 * # Chỉ định dạng, KHÔNG tính toán
 *
 * Backend luôn trả `{ amount, currency }` đã tính sẵn. Ở đây không có phép
 * cộng, không có "nếu giảm giá thì...", không quy đổi tiền tệ.
 *
 * Đặc biệt: KHÔNG tự cộng tổng giỏ hàng từ các dòng. Tổng do backend tính —
 * cộng lại ở đây nghĩa là hai nơi cùng tính một con số, và sớm muộn khách
 * thấy một số ở giỏ, một số khác ở bước thanh toán.
 */

interface Money {
  amount: number;
  currency: string;
}

/**
 * Hiển thị số tiền.
 *
 * `amount` là SỐ NGUYÊN theo đơn vị nhỏ nhất: VND là đồng, USD là cent.
 */
export function money(m: Money | undefined | null): string {
  if (!m) return "—";

  const minorUnits = m.currency === "VND" ? 0 : 2;
  const value = minorUnits === 0 ? m.amount : m.amount / 100;

  return new Intl.NumberFormat("vi-VN", {
    style: "currency",
    currency: m.currency,
    minimumFractionDigits: minorUnits,
  }).format(value);
}

/** Thời điểm ISO → giờ địa phương. */
export function dateTime(iso: string | undefined | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat("vi-VN", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(d);
}

/**
 * Thời gian còn lại của phiên thanh toán, dạng "12:34".
 *
 * Phiên giữ tồn kho 15 phút. Khách PHẢI thấy đồng hồ đếm ngược: hết hạn mà
 * không báo trước thì họ điền xong địa chỉ mới biết mình mất chỗ.
 */
export function countdown(expiresAt: string | undefined): string {
  if (!expiresAt) return "—";
  const ms = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(ms) || ms <= 0) return "00:00";

  const total = Math.floor(ms / 1000);
  const mm = String(Math.floor(total / 60)).padStart(2, "0");
  const ss = String(total % 60).padStart(2, "0");
  return `${mm}:${ss}`;
}

/**
 * Nhãn tiếng Việt cho tình trạng hàng.
 *
 * Món hết hàng được ĐÁNH DẤU chứ không bị xóa khỏi giỏ — nên nhãn phải nói
 * rõ khách cần làm gì, không chỉ "không khả dụng".
 */
export function availabilityLabel(a: string | undefined): string {
  switch (a) {
    case "IN_STOCK":
      return "Còn hàng";
    case "LOW_STOCK":
      return "Không đủ số lượng bạn chọn";
    case "OUT_OF_STOCK":
      return "Hết hàng";
    case "UNAVAILABLE":
      return "Không còn được bán";
    default:
      return "";
  }
}

/** Nhãn tiếng Việt cho trạng thái đơn hàng. */
export function orderStatusLabel(s: string | undefined): string {
  switch (s) {
    case "PENDING_PAYMENT":
      return "Chờ thanh toán";
    case "PAID":
      return "Đã thanh toán";
    case "PROCESSING":
      return "Đang chuẩn bị hàng";
    case "PARTIALLY_SHIPPED":
      return "Đã giao một phần";
    case "SHIPPED":
      return "Đang vận chuyển";
    case "PARTIALLY_DELIVERED":
      return "Đã nhận một phần";
    case "DELIVERED":
      return "Đã nhận hàng";
    case "COMPLETED":
      return "Hoàn tất";
    case "PARTIALLY_CANCELLED":
      return "Đã hủy một phần";
    case "CANCELLED":
      return "Đã hủy";
    default:
      return s ?? "—";
  }
}
