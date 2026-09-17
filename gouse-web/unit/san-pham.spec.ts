import { expect, test } from "@playwright/test";

import {
  maSKUGoiY,
  moTaBienThe,
  productStatusLabel,
  productTone,
  slugTu,
} from "../apps/seller/src/lib/san-pham";

test("slug bỏ dấu tiếng Việt, gồm cả chữ đ", () => {
  /**
   * `normalize("NFD")` tách dấu ra rồi xóa — xử đúng "ê", "ư", "ộ" mà
   * không cần bảng tra. Nhưng "đ" KHÔNG phải "d" kèm dấu, nó là một ký tự
   * riêng, nên phải thay tay. Bỏ sót nó là lỗi rất dễ mắc và chỉ lộ ra ở
   * đúng những từ tiếng Việt thường gặp nhất.
   */
  expect(slugTu("Áo sơ mi linen Oxford")).toBe("ao-so-mi-linen-oxford");
  expect(slugTu("Đầm dự tiệc")).toBe("dam-du-tiec");
  expect(slugTu("Quần jean ống rộng")).toBe("quan-jean-ong-rong");

  // Chữ "đ" THƯỜNG, ở giữa từ — ca này quan trọng hơn nó trông.
  //
  // Bản đầu của bài test chỉ có "Đầm" viết hoa, và xóa hẳn phép thay chữ
  // "đ" thường vẫn XANH: phép thay chữ "Đ" hoa chạy trước `toLowerCase`
  // nên nó gánh mất. Một bài test còn xanh khi xóa dòng nó sinh ra để giữ
  // là một bài test không làm việc.
  expect(slugTu("Áo len màu đỏ")).toBe("ao-len-mau-do");
  expect(slugTu("giày da đen")).toBe("giay-da-den");
});

test("slug không để lại gạch thừa ở đầu, cuối, hay giữa", () => {
  // Một slug như `-ao--len-` vẫn "chạy" và trông sai trong địa chỉ trang
  // sản phẩm — thứ sống lâu hơn mọi lần sửa sau này.
  expect(slugTu("  Áo   len  ")).toBe("ao-len");
  expect(slugTu("Áo / Len & Cotton")).toBe("ao-len-cotton");
  expect(slugTu("")).toBe("");
});

test("nháp BỊ TRẢ VỀ trông khác nháp mới viết", () => {
  /**
   * Miền KHÔNG có trạng thái `REJECTED`: `Reject` đưa sản phẩm về lại
   * `DRAFT` rồi ghi `rejection_reason`. Nhưng với nhà bán, hai tình huống
   * này khác hẳn nhau — một cái là việc chưa bắt đầu, một cái là việc bị
   * trả lại kèm lý do phải đọc.
   */
  expect(productStatusLabel("DRAFT")).toBe("Nháp");
  expect(productStatusLabel("DRAFT", "Ảnh mờ")).toBe("Bị trả về");
  expect(productTone("DRAFT")).toBe("warning");
  expect(productTone("DRAFT", "Ảnh mờ")).toBe("danger");
});

test("mọi trạng thái của miền đều có nhãn, và mã lạ không thành ô trống", () => {
  for (const s of ["DRAFT", "PENDING_REVIEW", "ACTIVE", "INACTIVE", "ARCHIVED"]) {
    expect(productStatusLabel(s), `mã ${s} chưa có nhãn`).not.toBe(s);
  }
  expect(productStatusLabel("TRANG_THAI_LA")).toBe("TRANG_THAI_LA");
  expect(productStatusLabel(undefined)).toBe("—");
});

test("mã SKU gợi ý đọc được và bỏ dấu", () => {
  /**
   * Mã SKU là thứ `inventory` đếm và người trong kho đọc. Bắt nhà bán tự
   * nghĩ ra quy ước đặt mã là cách chắc chắn để có `SP1`, `SP2`,
   * `test123` trong kho thật.
   */
  expect(maSKUGoiY("ao-so-mi-linen", "Trắng", "M")).toBe(
    "AO-SO-MI-LINEN-TRANG-M",
  );
  expect(maSKUGoiY("dam-lua", "Đỏ", "S")).toBe("DAM-LUA-DO-S");
});

test("mã SKU bỏ qua phần còn trống thay vì để lại gạch thừa", () => {
  // Người dùng gõ màu trước, size sau — giữa hai lần gõ, gợi ý không được
  // thành `AO--M`.
  expect(maSKUGoiY("ao-len", "", "M")).toBe("AO-LEN-M");
  expect(maSKUGoiY("ao-len", "Đen", "")).toBe("AO-LEN-DEN");
  expect(maSKUGoiY("", "", "")).toBe("");
});

test("mô tả biến thể GIẤU thuộc tính máy chủ tự suy ra", () => {
  /**
   * Backend thêm `color_family` từ tên màu ("Đỏ" → RED) để bộ lọc danh mục
   * dùng. Hiện nó thô cạnh "Đỏ" trông như lỗi lặp — và nó không phải thứ
   * nhà bán gõ vào nên không phải thứ họ sửa được.
   */
  expect(moTaBienThe({ color: "Đỏ", size: "M", color_family: "RED" })).toBe(
    "Đỏ / M",
  );

  // Thuộc tính HỢP LỆ khác vẫn hiện: giấu đi thì hai biến thể khác nhau
  // trông giống hệt nhau.
  expect(moTaBienThe({ color: "Đen", size: "M", fit: "oversize" })).toBe(
    "Đen / M / fit: oversize",
  );
  expect(moTaBienThe({})).toBe("—");
  expect(moTaBienThe(undefined)).toBe("—");
});
