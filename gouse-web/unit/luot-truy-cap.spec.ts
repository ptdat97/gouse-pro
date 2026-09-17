import { expect, test } from "@playwright/test";

import { maLuotTruyCap } from "../packages/api-client/src/luot-truy-cap";

/**
 * Mã LƯỢT TRUY CẬP là thứ nối nửa trên của phễu với nửa dưới.
 *
 * Nó bằng 0 không phải vì thiếu dữ liệu mà vì hai nửa đếm hai không gian
 * mã khác nhau — xem ADR-0020. Ba bài dưới đây giữ ba tính chất khiến nó
 * nối được; hỏng bất kỳ tính chất nào thì `conversion_rate` trở lại thành
 * một con số vô nghĩa, và nó vô nghĩa MỘT CÁCH IM LẶNG.
 */

/** Dựng `window` giả với một kho lưu trữ điều khiển được. */
function dungWindow(kho: Partial<Storage> | null): () => void {
  const truoc = (globalThis as Record<string, unknown>).window;
  (globalThis as Record<string, unknown>).window = kho
    ? { sessionStorage: kho }
    : {};
  return () => {
    (globalThis as Record<string, unknown>).window = truoc;
  };
}

function khoThat(): Partial<Storage> {
  const data = new Map<string, string>();
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  };
}

test("cùng một lượt thì trả CÙNG một mã", () => {
  const dung = dungWindow(khoThat());
  try {
    const a = maLuotTruyCap();
    const b = maLuotTruyCap();

    // Khác nhau nghĩa là mỗi lời gọi API thành một "lượt truy cập" riêng:
    // mẫu số phình lên bằng số request, và tỷ lệ chuyển đổi tiến về 0.
    expect(a).toBe(b);
    expect(a).not.toBe("");
    expect(a.startsWith("vis_")).toBe(true);
  } finally {
    dung();
  }
});

test("lưu ở sessionStorage, KHÔNG phải localStorage", () => {
  const kho = khoThat();
  const dung = dungWindow(kho);
  try {
    const ma = maLuotTruyCap();

    // sessionStorage mất khi đóng tab — đúng nghĩa MỘT LƯỢT. localStorage
    // sống hàng tháng, tức một NGƯỜI: dùng nó sẽ biến `conversion_rate`
    // thành "tỷ lệ người đã từng mua", một con số cao hơn và trả lời câu
    // hỏi khác.
    expect(kho.getItem?.("gouse_visit_id")).toBe(ma);
    expect(
      (globalThis as Record<string, unknown>).window,
    ).not.toHaveProperty("localStorage");
  } finally {
    dung();
  }
});

test("kho lưu trữ bị chặn thì trả RỖNG, không ném lỗi", () => {
  const dung = dungWindow({
    getItem: () => {
      throw new Error("chế độ riêng tư chặn lưu trữ");
    },
    setItem: () => {
      throw new Error("chế độ riêng tư chặn lưu trữ");
    },
  });
  try {
    // Đo đạc không phải lý do để trang hàng ngừng chạy. Và rỗng phải là
    // RỖNG chứ không phải một mã tạm: mã tạm khác nhau ở mỗi lần gọi sẽ
    // bơm số liệu rác vào đúng chỉ số nó phục vụ.
    expect(maLuotTruyCap()).toBe("");
  } finally {
    dung();
  }
});

test("chạy ở máy chủ (không có window) thì trả RỖNG", () => {
  const dung = dungWindow(null);
  (globalThis as Record<string, unknown>).window = undefined;
  try {
    // Next.js render trang lần đầu ở máy chủ. Ném lỗi ở đó là trang trắng.
    expect(maLuotTruyCap()).toBe("");
  } finally {
    dung();
  }
});
