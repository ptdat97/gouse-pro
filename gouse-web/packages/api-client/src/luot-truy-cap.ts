/**
 * Mã LƯỢT TRUY CẬP — thứ nối các việc khách làm trong một lần ghé thăm.
 *
 * # Nó KHÔNG phải định danh người
 *
 * Không gắn với tài khoản, không sống qua lần đóng tab, không đọc được bởi
 * máy chủ ở lần ghé sau. Nó chỉ trả lời một câu: "những việc này có xảy ra
 * trong cùng một lần ghé thăm không".
 *
 * Máy chủ CỐ Ý không tự sinh mã này. Mã sinh ở máy chủ sẽ khác nhau ở mỗi
 * request, nên mỗi lượt gọi thành một "lượt truy cập" riêng — và tỷ lệ
 * chuyển đổi tính trên nó là một con số vô nghĩa. Xem ADR-0020.
 *
 * # Vì sao sessionStorage, không phải localStorage
 *
 *	sessionStorage   sống theo TAB, mất khi đóng — đúng nghĩa "một lượt"
 *	localStorage     sống hàng tháng — đó là một NGƯỜI, không phải một lượt
 *
 * Dùng localStorage ở đây sẽ biến `conversion_rate` thành "tỷ lệ người đã
 * từng mua", một con số cao hơn và trả lời câu hỏi khác.
 */

const KHOA = "gouse_visit_id";

/**
 * maLuotTruyCap trả mã của lượt truy cập hiện tại, tạo nếu chưa có.
 *
 * Trả chuỗi RỖNG khi không có sessionStorage — chế độ riêng tư, trình
 * duyệt cũ, hoặc đang chạy ở máy chủ (Next.js render phía server). Rỗng là
 * hợp lệ: backend nhận request không có header và để trống trường phiên,
 * thay vì bịa một mã.
 */
export function maLuotTruyCap(): string {
  if (typeof window === "undefined") return "";

  try {
    const co = window.sessionStorage.getItem(KHOA);
    if (co) return co;

    const moi = sinhMa();
    window.sessionStorage.setItem(KHOA, moi);
    return moi;
  } catch {
    // sessionStorage ném lỗi khi trình duyệt chặn lưu trữ. Đo đạc không
    // phải lý do để trang hàng ngừng chạy.
    return "";
  }
}

function sinhMa(): string {
  const than =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID().replace(/-/g, "")
      : Math.random().toString(36).slice(2).padEnd(24, "0");
  return `vis_${than}`;
}
