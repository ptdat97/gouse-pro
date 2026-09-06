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

/**
 * Nhãn cho trạng thái offer.
 *
 * KHÔNG có "Hết hàng" ở đây: hết hàng không phải trạng thái của lời chào
 * bán mà là sự thật của tồn kho. Offer hết hàng vẫn `ACTIVE` — xem P3-23.
 */
export function offerStatusLabel(s: string | undefined): string {
  switch (s) {
    case "DRAFT":
      return "Nháp";
    case "ACTIVE":
      return "Đang bán";
    case "SUSPENDED":
      return "Bị tạm ngưng";
    case "ARCHIVED":
      return "Đã lưu trữ";
    default:
      return s ?? "—";
  }
}

/**
 * Nhãn và màu cho MỘT offer, tính từ cặp (`status`, `is_sellable`).
 *
 * # Vì sao phải là một CẶP
 *
 * "Hết hàng" không phải trạng thái của lời chào bán (xem `offerStatusLabel`
 * và P3-23), nên `status` một mình không bao giờ nói được điều đó: offer
 * hết hàng vẫn `ACTIVE`. Thứ nói được là `is_sellable`, câu trả lời máy chủ
 * đã tổng hợp từ trạng thái offer, tồn kho VÀ trạng thái nhà bán.
 *
 * Trước đây màn hình này chỉ đọc `status`, nên một offer hết sạch hàng vẫn
 * đeo huy hiệu xanh "Đang bán" — nhà bán không có cách nào biết mình đã
 * ngừng bán được hàng.
 *
 * # Không suy lại quy tắc ở đây
 *
 * `is_sellable` được DÙNG NGUYÊN, không ghép thêm điều kiện: đặc tả dặn
 * đúng điều đó, và chính máy chủ đã từng vi phạm nó. Giao diện chỉ DỊCH
 * câu trả lời sang chữ và màu.
 */
export function offerBadge(
  status: string | undefined,
  isSellable: boolean | undefined,
): { label: string; tone: Tone } {
  if (status !== "ACTIVE") {
    return { label: offerStatusLabel(status), tone: "neutral" };
  }
  if (isSellable) {
    return { label: "Đang bán", tone: "success" };
  }
  // ACTIVE mà không bán được: hết hàng, hoặc tài khoản nhà bán đang bị
  // đình chỉ. Giao diện KHÔNG đoán giữa hai lý do — nó không có dữ liệu để
  // phân biệt, và đoán sai thì nhà bán đi sửa nhầm chỗ.
  return { label: "Không bán được", tone: "warning" };
}
