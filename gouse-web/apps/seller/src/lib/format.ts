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

  const minorUnits = m.currency === "VND" ? 0 : 2;
  const value = minorUnits === 0 ? m.amount : m.amount / 100;

  return new Intl.NumberFormat("vi-VN", {
    style: "currency",
    currency: m.currency,
    minimumFractionDigits: minorUnits,
  }).format(value);
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
