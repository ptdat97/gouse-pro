/**
 * Chỉ số hiệu suất: nhãn, đơn vị, và HƯỚNG tốt.
 *
 * # Vì sao cần một bảng, không đoán theo tên
 *
 * API trả `{name: "cancellation_rate", value: 0.03, threshold: 0.03}` và
 * KHÔNG nói đơn vị. Hiện thẳng `0.03` cho một thứ nghĩa là 3% là sai ở mức
 * người đọc không nhận ra — con số trông hợp lệ.
 *
 * Suy đơn vị từ hậu tố `_rate` thì chạy được hôm nay và hỏng im lặng vào
 * ngày có chỉ số đầu tiên không phải tỷ lệ. Một bảng khai rõ thì chỉ số lạ
 * rơi vào nhánh mặc định và LỘ RA, thay vì được định dạng sai.
 *
 * # Hướng tốt cũng phải khai
 *
 * `ngưỡng 3%` một mình không nói được gì: 3% là sàn hay trần? Với tỷ lệ hủy
 * thì càng thấp càng tốt, với giao đúng hạn thì ngược lại. Hiện sai hướng
 * là nói với nhà bán rằng họ đang đạt trong khi họ đang vi phạm.
 */

type Huong = "caoTot" | "thapTot";

interface MoTaChiSo {
  nhan: string;
  donVi: "tyLe";
  huong: Huong;
  /** Vì sao chỉ số này tồn tại — nhà bán đọc được lý do mình bị chấm. */
  yNghia: string;
}

const BANG: Record<string, MoTaChiSo> = {
  cancellation_rate: {
    nhan: "Tỷ lệ hủy đơn",
    donVi: "tyLe",
    huong: "thapTot",
    yNghia: "Mỗi đơn hủy là một khách đã đặt rồi không nhận được hàng.",
  },
  on_time_shipping_rate: {
    nhan: "Giao đúng hạn",
    donVi: "tyLe",
    huong: "caoTot",
    yNghia: "Tính từ lúc đơn thực hiện được tạo, theo hạn bàn giao bên dưới.",
  },

  // Bốn chỉ số dưới đây CHƯA được chấm, nhưng vẫn cần nhãn: chúng xuất
  // hiện trong `not_measured`, và hiện mã thô `buy_box_win_rate` cho nhà
  // bán là bắt họ đoán.
  return_rate_description: {
    nhan: "Tỷ lệ hoàn vì mô tả sai",
    donVi: "tyLe",
    huong: "thapTot",
    yNghia: "Hàng về vì khác mô tả — dấu hiệu trang sản phẩm nói quá.",
  },
  average_rating: {
    nhan: "Điểm đánh giá trung bình",
    donVi: "tyLe",
    huong: "caoTot",
    yNghia: "Điểm khách chấm sau khi nhận hàng.",
  },
  inventory_accuracy: {
    nhan: "Độ chính xác tồn kho",
    donVi: "tyLe",
    huong: "caoTot",
    yNghia: "Số trên hệ thống khớp số thật trong kho.",
  },
  buy_box_win_rate: {
    nhan: "Tỷ lệ thắng ô mua",
    donVi: "tyLe",
    huong: "caoTot",
    yNghia: "Tỷ lệ bạn là người bán được hiển thị mặc định cho một SKU.",
  },
};

/** Nhãn tiếng Việt. Mã lạ trả về CHÍNH NÓ, không trả ô trống. */
export function chiSoNhan(ten: string | undefined): string {
  if (!ten) return "—";
  return BANG[ten]?.nhan ?? ten;
}

export function chiSoYNghia(ten: string | undefined): string {
  return (ten && BANG[ten]?.yNghia) ?? "";
}

/**
 * Giá trị đã định dạng.
 *
 * Chỉ số LẠ hiện số thô — đúng và nhìn rõ là lạ, thay vì nhân 100 một thứ
 * chưa chắc là tỷ lệ rồi gắn dấu %.
 */
export function chiSoGiaTri(ten: string | undefined, v: number | undefined): string {
  if (v === undefined || v === null || Number.isNaN(v)) return "—";
  if (ten && BANG[ten]?.donVi === "tyLe") {
    return `${(v * 100).toLocaleString("vi-VN", {
      maximumFractionDigits: 1,
    })}%`;
  }
  return v.toLocaleString("vi-VN");
}

/**
 * Ngưỡng kèm HƯỚNG: "≤ 3%" hay "≥ 95%".
 *
 * Không biết hướng thì nói "ngưỡng: X" chứ KHÔNG đoán một dấu — đoán sai
 * là nói ngược hoàn toàn về việc nhà bán có đạt hay không.
 */
export function chiSoNguong(ten: string | undefined, v: number | undefined): string {
  const so = chiSoGiaTri(ten, v);
  if (so === "—") return "—";
  const h = ten ? BANG[ten]?.huong : undefined;
  if (h === "thapTot") return `≤ ${so}`;
  if (h === "caoTot") return `≥ ${so}`;
  return `ngưỡng ${so}`;
}

export type Tone = "neutral" | "success" | "warning" | "danger" | "info";

export function chiSoTone(status: string | undefined): Tone {
  switch (status) {
    case "GOOD":
      return "success";
    case "WARNING":
      return "warning";
    case "CRITICAL":
      return "danger";
    default:
      return "info";
  }
}

export function chiSoTrangThaiNhan(status: string | undefined): string {
  switch (status) {
    case "GOOD":
      return "Đạt";
    case "WARNING":
      return "Cần chú ý";
    case "CRITICAL":
      // KHÔNG gọi là "Kém": chữ này đi kèm hậu quả thật (mất ô mua, bị
      // tạm ngưng), nên nó phải nghe như một cảnh báo chứ không như một
      // lời chê.
      return "Vi phạm ngưỡng";
    default:
      return status ?? "—";
  }
}

export const KY: { ma: "LAST_7_DAYS" | "LAST_30_DAYS" | "LAST_90_DAYS"; nhan: string }[] = [
  { ma: "LAST_7_DAYS", nhan: "7 ngày" },
  { ma: "LAST_30_DAYS", nhan: "30 ngày" },
  { ma: "LAST_90_DAYS", nhan: "90 ngày" },
];
