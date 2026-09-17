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

/**
 * Nhãn cho trạng thái yêu cầu trả hàng.
 *
 * `REFUNDED` là trạng thái CUỐI của tiền, không phải của hàng: tiền đã về
 * tay khách, nhưng món hàng vẫn nằm ở `Returned` cho tới khi kiểm định.
 * Gọi nó là "Hoàn tất" sẽ khiến nhà bán bỏ qua bước kiểm định và mất hàng.
 */
export function returnStatusLabel(s: string | undefined): string {
  switch (s) {
    case "REQUESTED":
      return "Chờ duyệt";
    case "APPROVED":
      return "Đã duyệt, chờ hàng về";
    case "REJECTED":
      return "Đã từ chối";
    case "RECEIVED":
      return "Đã nhận hàng";
    case "REFUNDED":
      return "Đã hoàn tiền";
    case "CANCELLED":
      return "Khách đã hủy";
    default:
      return s ?? "—";
  }
}

export function returnTone(s: string | undefined): Tone {
  switch (s) {
    case "REQUESTED":
      // Việc CẦN LÀM. Để lâu là khách chờ, và chờ trong lúc đang bực.
      return "warning";
    case "REFUNDED":
      return "success";
    case "REJECTED":
    case "CANCELLED":
      return "danger";
    default:
      return "info";
  }
}

/**
 * Nhãn lý do trả hàng.
 *
 * Mã CHUẨN HÓA chứ không phải văn bản tự do, và đó là cả điểm: hai mã
 * `SIZE_TOO_SMALL` đếm được thành một tín hiệu nhu cầu, hai câu "áo hơi
 * chật" thì không. Ở đây chỉ dịch sang tiếng người.
 */
export function returnReasonLabel(s: string | undefined): string {
  switch (s) {
    case "SIZE_TOO_SMALL":
      return "Size quá chật";
    case "SIZE_TOO_LARGE":
      return "Size quá rộng";
    case "NOT_AS_DESCRIBED":
      return "Khác mô tả";
    case "COLOR_DIFFERENT":
      return "Màu khác ảnh";
    case "QUALITY_ISSUE":
      return "Chất lượng không như kỳ vọng";
    case "DEFECTIVE":
      return "Hàng lỗi";
    case "WRONG_ITEM_SENT":
      return "Giao sai món";
    case "DAMAGED_IN_TRANSIT":
      return "Hỏng khi vận chuyển";
    case "CHANGED_MIND":
      return "Đổi ý";
    case "LATE_DELIVERY":
      return "Giao quá chậm";
    default:
      // Mã lạ KHÔNG được làm crash, và cũng không được hiện thành khoảng
      // trắng: đặc tả bắt enum mới phải không làm hỏng giao diện, và một
      // ô trống khiến người trực không biết khách than gì.
      return s ?? "—";
  }
}
