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

  // CHUẨN HÓA chữ hoa trước khi so sánh.
  //
  // So sánh phân biệt hoa thường ở đây từng cho ra lỗi tệ nhất mà một
  // trang bán hàng có thể mắc: `"vnd"` không khớp `"VND"`, nên số tiền bị
  // chia cho 100 và 250.000 ₫ hiện thành 2.500 ₫ — vẫn kèm ký hiệu ₫ nên
  // trông hoàn toàn hợp lệ.
  //
  // Backend đã chuẩn hóa từ 04/09, nhưng giữ ở đây làm lớp thứ hai: giao
  // diện không được hiện sai GIÁ dù nhận vào cái gì.
  const currency = (m.currency ?? "").toUpperCase();

  const minorUnits = currency === "VND" ? 0 : 2;
  const value = minorUnits === 0 ? m.amount : m.amount / 100;

  try {
    return new Intl.NumberFormat("vi-VN", {
      style: "currency",
      currency,
      minimumFractionDigits: minorUnits,
    }).format(value);
  } catch {
    // `Intl.NumberFormat` NÉM RangeError với mã tiền tệ không hợp lệ, và
    // một lỗi ném lúc render làm TRẮNG cả trang thay vì hiện một ô sai.
    //
    // Hiện con số kèm mã thô: người dùng vẫn đọc được, và mã lạ nằm ngay
    // đó cho người vận hành thấy có gì sai.
    return `${value.toLocaleString("vi-VN")} ${currency || "?"}`.trim();
  }
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
/**
 * Mili giây còn lại tới `expiresAt`, hoặc `null` khi KHÔNG BIẾT.
 *
 * Một nguồn sự thật duy nhất cho cả đồng hồ đếm ngược lẫn cờ "đã hết hạn".
 * Hai chỗ tự tính riêng thì chúng bất đồng được — và đã bất đồng: trang
 * thanh toán tính `new Date(...).getTime() - now` rồi so `<= 0`, trong khi
 * `countdown` gộp giá trị hỏng thành "00:00".
 *
 * `null` KHÁC 0: không biết thì bên gọi tự quyết, chứ không bị ép coi là
 * đã hết hạn.
 */
export function msConLai(expiresAt: string | undefined): number | null {
  if (!expiresAt) return null;
  const moc = new Date(expiresAt).getTime();
  if (Number.isNaN(moc)) return null;
  return moc - Date.now();
}

/**
 * Thời gian còn lại của phiên thanh toán, dạng "12:34".
 *
 * Phiên giữ tồn kho 15 phút. Khách PHẢI thấy đồng hồ đếm ngược: hết hạn mà
 * không báo trước thì họ điền xong địa chỉ mới biết mình mất chỗ.
 *
 * # "—" và "00:00" nói hai chuyện KHÁC nhau
 *
 * `null` là KHÔNG BIẾT (thiếu mốc, hoặc mốc không đọc được); `00:00` là ĐÃ
 * HẾT HẠN. Bản đầu gộp cả hai thành "00:00", và điều đó tạo ra một trang
 * tự mâu thuẫn: `expires_at` hỏng làm `msLeft` thành NaN, mà `NaN <= 0` là
 * false — nên trang coi phiên CHƯA hết hạn, cho bấm tiếp, trong khi đồng
 * hồ hiện "00:00".
 *
 * Khách đọc được "Hàng được giữ cho bạn trong 00:00" và không biết tin cái
 * nào.
 */
export function countdown(expiresAt: string | undefined): string {
  const ms = msConLai(expiresAt);
  if (ms === null) return "—";
  if (ms <= 0) return "00:00";

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

/**
 * Nhãn tiếng Việt cho trạng thái một GÓI GIAO.
 *
 * Khác `orderStatusLabel`: đó là trạng thái TỔNG HỢP của cả đơn, còn đây là
 * của từng gói. Khách có thể thấy "Đã giao" ở một gói và "Đang đóng gói" ở
 * gói khác trong cùng một đơn — dùng chung một bảng nhãn sẽ nói sai.
 */
export function shipmentStatusLabel(s: string | undefined): string {
  switch (s) {
    case "PENDING":
      return "Chờ xử lý";
    case "ALLOCATED":
      return "Đã phân bổ kho";
    case "CONFIRMED":
      return "Nhà bán đã xác nhận";
    case "PICKING":
      return "Đang lấy hàng";
    case "PACKED":
      return "Đã đóng gói";
    case "HANDED_OVER":
      return "Đã bàn giao vận chuyển";
    case "IN_TRANSIT":
      return "Đang vận chuyển";
    case "DELIVERED":
      return "Đã giao";
    case "DELIVERY_FAILED":
      return "Giao không thành công";
    case "COMPLETED":
      return "Hoàn tất";
    case "CANCELLED":
      return "Đã hủy";
    default:
      return s ?? "—";
  }
}
