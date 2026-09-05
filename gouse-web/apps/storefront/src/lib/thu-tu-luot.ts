/**
 * Giữ THỨ TỰ cho các lời gọi mạng chồng nhau.
 *
 * # Vấn đề nó giải
 *
 * Trang vừa tải nên `GET /cart` đang chạy chậm. Khách bấm "Thêm vào giỏ".
 * Lời gọi thêm xong TRƯỚC. Rồi `GET /cart` cũ về sau và ghi đè bằng giỏ
 * CHƯA có món vừa thêm.
 *
 * Khách thấy món mình vừa thêm biến mất, dù máy chủ đã ghi nhận. Họ bấm
 * thêm lần nữa — và giờ có hai món trong giỏ.
 *
 * Đây đúng hình dạng lỗi "đọc-rồi-ghi" ở backend, chỉ khác là cuộc đua nằm
 * giữa hai phản hồi MẠNG chứ không phải hai giao dịch database.
 *
 * # Vì sao tách khỏi component
 *
 * Quy tắc "chỉ nhận kết quả của lượt mới nhất" là logic thuần, và kiểm nó
 * không cần dựng cây React. Để nó lẫn trong `useCallback` thì muốn kiểm
 * phải render cả provider, giả lập `fetch`, và điều khiển thời điểm phản
 * hồi — nhiều công hơn hẳn, cho cùng một câu trả lời.
 */
export class ThuTuLuot {
  #moiNhat = 0;

  /** Bắt đầu một lượt; trả về hàm hỏi "lượt này còn mới nhất không". */
  batDau(): () => boolean {
    const luot = ++this.#moiNhat;
    return () => luot === this.#moiNhat;
  }
}
