/**
 * Nhãn và màu cho trạng thái đơn thực hiện.
 *
 * Tách khỏi component vì cùng một trạng thái xuất hiện ở nhiều màn hình, và
 * hai chỗ dịch khác nhau thì nhà bán tưởng là hai việc khác nhau.
 */

type Tone = "neutral" | "success" | "warning" | "danger" | "info";

export function foStatusLabel(s: string | undefined): string {
  switch (s) {
    case "PENDING":
      return "Chờ xử lý";
    case "ALLOCATED":
      return "Đã phân bổ kho";
    case "CONFIRMED":
      return "Đã xác nhận";
    case "PICKING":
      return "Đang lấy hàng";
    case "PACKED":
      return "Đã đóng gói";
    case "HANDED_OVER":
      return "Đã bàn giao";
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

export function foTone(s: string | undefined): Tone {
  switch (s) {
    case "PENDING":
      // Việc CẦN LÀM, không phải lỗi — nhưng phải nổi bật hơn trạng thái
      // trung tính, vì để lâu là trễ cam kết giao hàng.
      return "warning";
    case "DELIVERED":
    case "COMPLETED":
      return "success";
    case "DELIVERY_FAILED":
    case "CANCELLED":
      return "danger";
    default:
      return "info";
  }
}

/** Nhãn cho trạng thái offer. */
export function offerStatusLabel(s: string | undefined): string {
  switch (s) {
    case "DRAFT":
      return "Nháp";
    case "ACTIVE":
      return "Đang bán";
    case "OUT_OF_STOCK":
      return "Hết hàng";
    case "SUSPENDED":
      return "Bị tạm ngưng";
    case "ARCHIVED":
      return "Đã lưu trữ";
    default:
      return s ?? "—";
  }
}
