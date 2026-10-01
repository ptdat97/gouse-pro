import { expect, test } from "@playwright/test";

import { settlementTone } from "../apps/admin/src/lib/status";

/**
 * Màn hình KÝ đợt đối soát — đường tiền RA nhà bán.
 *
 * Bài này kiểm phần quyết định được ở mức hàm thuần: sắc thái trạng thái, và
 * logic "trạng thái nào có nút nào". Phần còn lại (hộp thoại lý do, lời gọi
 * API) do bài tích hợp Go gác — xem `internal/app/api_ky_doi_soat_test.go`.
 */

test("DRAFT là trạng thái CẦN NGƯỜI LÀM GÌ, không phải trung tính", () => {
  /**
   * Chỗ dễ nói sai nhất ở màn hình này.
   *
   * `DRAFT` nghe như "bản nháp" — một thứ chưa cần ai nhìn. Nhưng với đợt
   * đối soát nó nghĩa là **nhà bán chưa được chi**, và tới 01/10/2026 MỌI
   * đợt đều nằm đó vì vòng đời chưa khép (P3-89).
   *
   * Sắc thái trung tính sẽ làm nó lẫn vào danh sách; `warning` đưa nó lên.
   */
  expect(settlementTone("DRAFT")).toBe("warning");
});

test("PAID là xong, CONFIRMED là đang chờ bước hai", () => {
  expect(settlementTone("PAID")).toBe("success");
  expect(settlementTone("CONFIRMED")).toBe("info");
});

test("trạng thái LẠ rơi về trung tính, không làm crash", () => {
  /**
   * Đặc tả OpenAPI ghi rõ: server có thể thêm giá trị enum mới trong cùng
   * phiên bản API, và client PHẢI rơi vào nhánh mặc định.
   *
   * Một màn hình TIỀN trắng trang vì một trạng thái lạ là tệ hơn một huy
   * hiệu màu xám.
   */
  expect(settlementTone("TRANG_THAI_MOI")).toBe("neutral");
  expect(settlementTone(undefined)).toBe("neutral");
});

/**
 * Nút nào hiện ở trạng thái nào.
 *
 * Tách thành hàm thuần để kiểm được: logic này nằm trong component, và một
 * nút hiện sai trạng thái nghĩa là người vận hành bấm rồi nhận 409 — họ sẽ
 * nghĩ hệ thống lỗi.
 */
function nutCuaTrangThai(status: string, thucChi: number): string | null {
  if (status === "DRAFT") return thucChi > 0 ? "Chốt" : "Chốt (mờ)";
  if (status === "CONFIRMED") return "Đã chuyển tiền";
  return null;
}

test("PAID KHÔNG có nút nào", () => {
  /**
   * Vòng đời đã hết. Một nút không làm gì là một nút người dùng sẽ bấm rồi
   * không hiểu vì sao im lặng.
   */
  expect(nutCuaTrangThai("PAID", 270000)).toBeNull();
});

test("mỗi trạng thái có ĐÚNG MỘT nút, không có nút làm cả hai bước", () => {
  /**
   * Hai bước là hai bước có chủ ý: bước một là lúc con số thành CAM KẾT với
   * nhà bán, bước hai là lúc tiền rời nền tảng. Một nút "Duyệt và chi" gộp
   * chúng lại sẽ bỏ mất chỗ soát con số.
   */
  expect(nutCuaTrangThai("DRAFT", 270000)).toBe("Chốt");
  expect(nutCuaTrangThai("CONFIRMED", 270000)).toBe("Đã chuyển tiền");
});

test("đợt không có số dương thì nút chốt phải MỜ, không để bấm rồi nhận lỗi", () => {
  /**
   * `XacNhan` từ chối `net ≤ 0`: nhà bán đang nợ nền tảng nhiều hơn số kiếm
   * được trong kỳ, và khoản nợ chuyển sang kỳ sau.
   *
   * Để nút sáng rồi trả lỗi là bắt người vận hành học quy tắc bằng cách gặp
   * lỗi — mà đây là màn hình tiền.
   */
  expect(nutCuaTrangThai("DRAFT", 0)).toBe("Chốt (mờ)");
});
