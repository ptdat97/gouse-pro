/**
 * Ánh xạ trạng thái sang sắc thái hiển thị.
 *
 * # Giá trị lạ KHÔNG được làm crash
 *
 * Đặc tả OpenAPI ghi rõ: server có thể thêm giá trị enum mới trong cùng
 * phiên bản API, và client PHẢI rơi vào nhánh mặc định. Vì thế mọi hàm ở
 * đây nhận `string`, không nhận union — union sẽ khiến TypeScript cho qua
 * nhưng runtime nhận giá trị không có trong bản đồ.
 */

type Tone = "neutral" | "success" | "warning" | "danger" | "info";

const SELLER_TONES: Record<string, Tone> = {
  APPLIED: "neutral",
  PENDING_REVIEW: "warning",
  APPROVED: "info",
  ACTIVE: "success",
  REJECTED: "danger",
  SUSPENDED: "danger",
  ON_VACATION: "neutral",
  TERMINATED: "danger",
};

const ORDER_TONES: Record<string, Tone> = {
  PENDING_PAYMENT: "warning",
  PAID: "info",
  PROCESSING: "info",
  SHIPPED: "info",
  DELIVERED: "success",
  COMPLETED: "success",
  CANCELLED: "danger",
  REFUNDED: "neutral",
};

// Nhận `undefined` vì kiểu sinh từ OpenAPI cho biết `status` không phải
// trường bắt buộc. Ép kiểu để làm TypeScript im lặng chỉ giấu vấn đề tới
// lúc chạy.
export function sellerTone(status: string | undefined): Tone {
  return status ? SELLER_TONES[status] ?? "neutral" : "neutral";
}

export function orderTone(status: string | undefined): Tone {
  return status ? ORDER_TONES[status] ?? "neutral" : "neutral";
}
