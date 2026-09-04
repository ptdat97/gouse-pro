/**
 * Định dạng để HIỂN THỊ.
 *
 * # Chỉ định dạng, KHÔNG tính toán
 *
 * Backend luôn trả `{ amount, currency }` đã tính sẵn. Ở đây không có phép
 * cộng, không có "nếu giảm giá thì...", không quy đổi tiền tệ.
 *
 * Quy đổi tiền tệ là NGHIỆP VỤ (cần tỷ giá tại thời điểm giao dịch), không
 * phải trình bày — xem frontend-architecture.md mục 10.
 */

interface Money {
  amount: number;
  currency: string;
}

/**
 * Hiển thị số tiền.
 *
 * `amount` là SỐ NGUYÊN theo đơn vị nhỏ nhất: VND là đồng, USD là cent.
 * Chia cho 100 với USD là việc của định dạng, không phải của tính toán.
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

/** Phần vạn → phần trăm. 1000 = "10,00%". */
export function basisPoints(bp: number | undefined): string {
  if (bp === undefined) return "—";
  return `${(bp / 100).toLocaleString("vi-VN", {
    minimumFractionDigits: 2,
  })}%`;
}

/** Thời điểm ISO → giờ địa phương, dạng người đọc được. */
export function dateTime(iso: string | undefined | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat("vi-VN", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(d);
}

/** Rút gọn định danh dài để bảng không bị vỡ. Giữ tiền tố để còn nhận ra. */
export function shortId(id: string | undefined): string {
  if (!id) return "—";
  const [prefix, rest] = id.split("_");
  if (!rest) return id;
  return `${prefix}_${rest.slice(0, 6)}…`;
}
