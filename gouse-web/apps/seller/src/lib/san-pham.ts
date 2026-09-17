/**
 * Nhãn và tiện ích cho luồng đăng sản phẩm.
 */

type Tone = "neutral" | "success" | "warning" | "danger" | "info";

/**
 * Trạng thái sản phẩm, nói theo góc nhìn NHÀ BÁN.
 *
 * # KHÔNG có trạng thái `REJECTED`
 *
 * Bản đầu của tệp này khai một nhãn cho `REJECTED`, và TypeScript từ chối
 * biên dịch: `domain.Status` chỉ có năm giá trị và không có cái đó.
 *
 * Mô hình thật khác: `Reject` đưa sản phẩm VỀ LẠI `DRAFT` rồi ghi
 * `rejection_reason`. Hợp lý — bị từ chối nghĩa là quay lại bàn làm việc,
 * không phải một ngõ cụt riêng.
 *
 * Nhưng với nhà bán, "nháp mới viết" và "nháp vừa bị trả về" là hai tình
 * huống khác hẳn nhau, nên nhãn tính từ CẢ HAI trường thay vì chỉ `status`.
 */
export function productStatusLabel(
  s: string | undefined,
  lyDoTuChoi?: string | null,
): string {
  if (s === "DRAFT" && lyDoTuChoi) return "Bị trả về";
  switch (s) {
    case "DRAFT":
      return "Nháp";
    case "PENDING_REVIEW":
      return "Chờ duyệt";
    case "ACTIVE":
      return "Đang bán";
    case "INACTIVE":
      return "Tạm ngừng bán";
    case "ARCHIVED":
      return "Đã lưu trữ";
    default:
      return s ?? "—";
  }
}

export function productTone(
  s: string | undefined,
  lyDoTuChoi?: string | null,
): Tone {
  // Bị trả về là việc CẦN LÀM: có lý do để đọc và có thể gửi lại ngay.
  if (s === "DRAFT" && lyDoTuChoi) return "danger";
  switch (s) {
    case "ACTIVE":
      return "success";
    case "PENDING_REVIEW":
      return "info";
    case "DRAFT":
      return "warning";
    case "INACTIVE":
      return "neutral";
    default:
      return "neutral";
  }
}

export const LOAI_SAN_PHAM = [
  { ma: "TOP", nhan: "Áo" },
  { ma: "BOTTOM", nhan: "Quần / chân váy" },
  { ma: "DRESS", nhan: "Đầm" },
  { ma: "OUTERWEAR", nhan: "Áo khoác" },
  { ma: "SHOES", nhan: "Giày" },
  { ma: "BAG", nhan: "Túi" },
  { ma: "ACCESSORY", nhan: "Phụ kiện" },
];

export const GIOI_TINH = [
  { ma: "WOMEN", nhan: "Nữ" },
  { ma: "MEN", nhan: "Nam" },
  { ma: "UNISEX", nhan: "Unisex" },
  { ma: "KIDS", nhan: "Trẻ em" },
];

/**
 * Tên tiếng Việt → slug không dấu.
 *
 * # Vì sao làm ở giao diện
 *
 * Bắt nhà bán tự gõ một chuỗi không dấu, không khoảng trắng là bước dễ bỏ
 * nhất của cả biểu mẫu — và một slug gõ vội ("ao-so-mi-2") sống mãi trong
 * địa chỉ trang sản phẩm.
 *
 * Đây là GỢI Ý, không phải quy tắc: người dùng sửa được, và backend vẫn là
 * nơi quyết định slug có hợp lệ và có trùng hay không.
 *
 * `normalize("NFD")` tách dấu thành ký tự riêng rồi xóa chúng — cách này
 * xử đúng cả "ê", "ư", "ộ" mà không cần bảng tra. Riêng "đ" không phải chữ
 * "d" kèm dấu nên phải thay riêng.
 */
export function slugTu(ten: string): string {
  return ten
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/đ/g, "d")
    .replace(/Đ/g, "D")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/**
 * KHÓA thuộc tính là CHUẨN CHUNG, không phải do nhà bán đặt.
 *
 * `variant.go` nói rõ vì sao: *"Nếu mỗi seller tự đặt tên khóa ('color',
 * 'colour', 'mau_sac'), bộ lọc theo màu sẽ vô dụng."*
 *
 * Nên biểu mẫu cho các Ô CỐ ĐỊNH thay vì để nhà bán tự nhập cặp khóa–giá
 * trị. Ràng buộc được cưỡng chế bằng hình dạng giao diện, không bằng một
 * dòng hướng dẫn mà ai cũng bỏ qua.
 */
export const KHOA_MAU = "color";
export const KHOA_SIZE = "size";

/**
 * Gợi ý mã SKU từ slug sản phẩm và tổ hợp thuộc tính.
 *
 * Mã SKU là thứ `inventory` đếm và người trong kho đọc, nên nó nên nói
 * được món hàng là gì. Bắt nhà bán tự nghĩ ra một quy ước đặt mã là cách
 * chắc chắn để có `SP1`, `SP2`, `test123` trong kho thật.
 *
 * Đây là GỢI Ý: sửa được, và backend vẫn từ chối mã trùng (409).
 */
export function maSKUGoiY(slug: string, mau: string, size: string): string {
  const phan = [slug, mau, size]
    .map((x) => slugTu(x))
    .filter(Boolean)
    .join("-");
  return phan.toUpperCase();
}

/**
 * Thuộc tính do MÁY CHỦ suy ra, không phải nhà bán nhập.
 *
 * `color_family` sinh từ tên màu: "Đỏ" → `RED`. Khách lọc theo "màu đỏ",
 * không lọc theo "Đỏ đô", nên nhóm màu là thứ bộ lọc dùng (`color.go`).
 *
 * Nó CÓ trong dữ liệu trả về và không nên hiện như một thuộc tính nhà bán
 * gõ vào — hiện thô `color_family: RED` cạnh "Đỏ" trông như lỗi lặp.
 */
const SUY_RA = new Set(["color_family", "color_hex"]);

/** "Đen / M" — mô tả một biến thể theo cách người đọc hiểu ngay. */
export function moTaBienThe(attrs: Record<string, string> | undefined): string {
  if (!attrs) return "—";
  const thuTu = [KHOA_MAU, KHOA_SIZE];
  const phan: string[] = [];
  for (const k of thuTu) {
    if (attrs[k]) phan.push(attrs[k]);
  }
  // Thuộc tính NGOÀI hai khóa chuẩn vẫn hiện — chúng hợp lệ (material,
  // pattern, fit), và giấu đi thì hai biến thể khác nhau trông giống hệt.
  // Trừ những thuộc tính máy chủ tự suy ra.
  for (const [k, v] of Object.entries(attrs)) {
    if (!thuTu.includes(k) && !SUY_RA.has(k) && v) phan.push(`${k}: ${v}`);
  }
  return phan.length ? phan.join(" / ") : "—";
}
