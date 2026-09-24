import type { NhomMau } from "@fc/api-client";

/**
 * Nhãn tiếng Việt và ô màu cho từng NHÓM màu.
 *
 * # Vì sao lọc theo nhóm, không theo tên màu
 *
 * Khách lọc "màu xanh", không lọc "Xanh navy đậm". Một sàn thời trang có
 * hàng trăm tên màu do người bán tự đặt — "Trắng ngà", "Trắng kem",
 * "Off-white" — và bộ lọc liệt kê từng cái là bộ lọc không ai dùng.
 *
 * Máy chủ SUY RA nhóm từ tên màu (`domain.SuyRaNhomMau`), nên người bán
 * không phải khai thêm gì.
 *
 * # `Record<NhomMau, …>` là HÀNG RÀO, không phải trang trí
 *
 * `NhomMau` suy từ `ColorFamily` trong đặc tả. Thêm một nhóm vào đặc tả mà
 * quên nhãn ở đây thì `npm run typecheck` ĐỎ — không phải một ô trống mà
 * khách nhìn thấy trước.
 *
 * Chiều ngược lại cũng được gác: bỏ một nhóm khỏi đặc tả thì khóa thừa ở
 * đây thành lỗi biên dịch. Đó là điều một danh sách chép tay không làm
 * được, và là lý do bài test kiểu "mọi mã đều có nhãn" không cần tồn tại
 * cho bảng này.
 */
export const NHOM_MAU: Record<NhomMau, { nhan: string; o: string }> = {
  WHITE: { nhan: "Trắng", o: "var(--swatch-white)" },
  BLACK: { nhan: "Đen", o: "var(--swatch-black)" },
  GREY: { nhan: "Xám", o: "var(--swatch-grey)" },
  SILVER: { nhan: "Bạc", o: "var(--swatch-silver)" },
  BEIGE: { nhan: "Be", o: "var(--swatch-beige)" },
  BROWN: { nhan: "Nâu", o: "var(--swatch-brown)" },
  RED: { nhan: "Đỏ", o: "var(--swatch-red)" },
  PINK: { nhan: "Hồng", o: "var(--swatch-pink)" },
  ORANGE: { nhan: "Cam", o: "var(--swatch-orange)" },
  YELLOW: { nhan: "Vàng", o: "var(--swatch-yellow)" },
  GREEN: { nhan: "Xanh lá", o: "var(--swatch-green)" },
  BLUE: { nhan: "Xanh dương", o: "var(--swatch-blue)" },
  PURPLE: { nhan: "Tím", o: "var(--swatch-purple)" },
  OTHER: { nhan: "Nhiều màu", o: "var(--swatch-other)" },
};

/**
 * Thứ tự hiển thị — theo MẮT, không theo bảng chữ cái.
 *
 * Trung tính trước (khách mua đồ cơ bản tìm chúng nhiều nhất), rồi ấm sang
 * lạnh. "Nhiều màu" xuống cuối vì nó là nhóm gom phần còn lại.
 *
 * Kiểu `NhomMau[]` nên gõ sai một nhóm là lỗi biên dịch; nhưng THIẾU một
 * nhóm thì không — xem `nhomMauHienThi`.
 */
const THU_TU: NhomMau[] = [
  "BLACK", "WHITE", "GREY", "SILVER", "BEIGE", "BROWN",
  "RED", "PINK", "ORANGE", "YELLOW", "GREEN", "BLUE", "PURPLE", "OTHER",
];

/**
 * Danh sách nhóm màu để dựng bộ lọc, đã sắp xếp.
 *
 * Nhóm có trong `NHOM_MAU` mà thiếu trong `THU_TU` vẫn được hiện — ở cuối.
 * Bỏ sót một dòng trong bảng thứ tự là chuyện sẽ xảy ra; hậu quả của nó
 * phải là "màu ấy nằm sai chỗ", không phải "khách không lọc được màu ấy".
 */
export function nhomMauHienThi(): NhomMau[] {
  const conLai = (Object.keys(NHOM_MAU) as NhomMau[]).filter(
    (m) => !THU_TU.includes(m),
  );
  return [...THU_TU, ...conLai];
}

/**
 * Đọc bộ lọc màu từ query string.
 *
 * # Giá trị LẠ bị bỏ, không làm hỏng trang
 *
 * `?color=CAM_VANG` là thứ ai cũng gõ được vào thanh địa chỉ. Gửi thẳng
 * lên máy chủ thì nó trả rỗng và khách thấy "không có sản phẩm nào" mà
 * không hiểu vì sao. Lọc ở đây để URL hỏng chỉ đơn giản là không lọc.
 */
export function docMauTuURL(raw: string | null): NhomMau[] {
  if (!raw) return [];
  const hopLe = new Set(Object.keys(NHOM_MAU));
  return raw
    .split(",")
    .map((x) => x.trim().toUpperCase())
    .filter((x) => hopLe.has(x)) as NhomMau[];
}
