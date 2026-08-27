import { expect, test } from "@playwright/test";

import {
  chonBanDau,
  doiMau,
  gomTheoMau,
  type MotMau,
} from "../apps/storefront/src/lib/chon-hang";

/**
 * Dữ liệu giống hệt hình dạng API thật trả về: danh sách biến thể PHẲNG,
 * trong đó cùng một màu xuất hiện nhiều lần — mỗi lần một size.
 */
function bienThe(color: string, size: string, skuId: string, anh?: string) {
  return {
    id: `var_${skuId}`,
    color,
    images: anh ? [{ url: anh }] : [],
    skus: [{ id: skuId, sku_code: `CODE-${skuId}`, size }],
  };
}

function offer(skuId: string, sellable: boolean) {
  return {
    id: `off_${skuId}${sellable ? "" : "_x"}`,
    sku_id: skuId,
    seller_id: "sel_1",
    price: { amount: 100, currency: "VND" },
    condition: "NEW" as const,
    handling_time_hours: 24,
    is_buy_box: true,
    is_sellable: sellable,
    status: sellable ? ("ACTIVE" as const) : ("OUT_OF_STOCK" as const),
  };
}

test("gom biến thể phẳng thành cây màu → size", () => {
  const mau = gomTheoMau(
    [
      bienThe("Trắng", "M", "sku_wm", "anh-trang.jpg"),
      bienThe("Trắng", "L", "sku_wl"),
      bienThe("Xanh navy", "M", "sku_nm", "anh-xanh.jpg"),
    ],
    [offer("sku_wm", true), offer("sku_wl", true), offer("sku_nm", true)],
  );

  expect(mau.map((m) => m.mau)).toEqual(["Trắng", "Xanh navy"]);
  expect(mau[0]!.sizes.map((s) => s.size)).toEqual(["M", "L"]);
  expect(mau[1]!.sizes.map((s) => s.size)).toEqual(["M"]);

  // Ảnh là thuộc tính của MÀU. Biến thể "Trắng/L" không có ảnh riêng,
  // nhưng màu Trắng vẫn phải có ảnh từ biến thể đầu tiên.
  expect(mau[0]!.anh).toBe("anh-trang.jpg");
  expect(mau[1]!.anh).toBe("anh-xanh.jpg");
});

test("còn hàng suy từ OFFER, không phải từ việc SKU có tồn tại", () => {
  const mau = gomTheoMau(
    [bienThe("Trắng", "M", "sku_wm"), bienThe("Trắng", "L", "sku_wl")],
    [
      offer("sku_wm", true),
      // Size L CÓ offer nhưng offer đó không bán được.
      offer("sku_wl", false),
    ],
  );

  expect(mau[0]!.sizes.find((s) => s.size === "M")!.coHang).toBe(true);
  expect(mau[0]!.sizes.find((s) => s.size === "L")!.coHang).toBe(false);

  // Size hết hàng VẪN có mặt. Ẩn đi thì khách tưởng thương hiệu không làm
  // size của họ, và bỏ đi thay vì đăng ký nhận thông báo.
  expect(mau[0]!.sizes).toHaveLength(2);
});

test("size KHÔNG có offer nào cũng hiện ra, và là hết hàng", () => {
  const mau = gomTheoMau(
    [bienThe("Trắng", "M", "sku_wm"), bienThe("Trắng", "XL", "sku_wxl")],
    [offer("sku_wm", true)],
  );

  const xl = mau[0]!.sizes.find((s) => s.size === "XL");
  expect(xl, "size không có nhà bán nào chào vẫn phải hiện").toBeTruthy();
  expect(xl!.coHang).toBe(false);
});

test("màu hết sạch hàng vẫn hiện, nhưng đánh dấu hết", () => {
  const mau = gomTheoMau(
    [bienThe("Trắng", "M", "sku_wm"), bienThe("Đen", "M", "sku_bm")],
    [offer("sku_wm", true), offer("sku_bm", false)],
  );

  expect(mau.map((m) => m.coHang)).toEqual([true, false]);
});

test("chọn sẵn tổ hợp MUA ĐƯỢC, không phải cái đầu tiên", () => {
  const mau = gomTheoMau(
    [
      // Màu đầu tiên hết sạch.
      bienThe("Trắng", "M", "sku_wm"),
      bienThe("Xanh navy", "M", "sku_nm"),
    ],
    [offer("sku_wm", false), offer("sku_nm", true)],
  );

  expect(chonBanDau(mau)).toEqual({ mau: "Xanh navy", skuId: "sku_nm" });
});

test("size đầu tiên hết hàng thì chọn size còn hàng trong cùng màu", () => {
  const mau = gomTheoMau(
    [bienThe("Trắng", "M", "sku_wm"), bienThe("Trắng", "L", "sku_wl")],
    [offer("sku_wm", false), offer("sku_wl", true)],
  );

  expect(chonBanDau(mau)).toEqual({ mau: "Trắng", skuId: "sku_wl" });
});

test("hết sạch hàng vẫn chọn sẵn một tổ hợp, không để trống", () => {
  const mau = gomTheoMau(
    [bienThe("Trắng", "M", "sku_wm")],
    [offer("sku_wm", false)],
  );

  // Để trống thì trang trông như chưa tải xong, thay vì nói rõ "hết hàng".
  expect(chonBanDau(mau)).toEqual({ mau: "Trắng", skuId: "sku_wm" });
});

test("đổi màu thì GIỮ NGUYÊN size nếu màu mới có size đó", () => {
  const mau = gomTheoMau(
    [
      bienThe("Trắng", "M", "sku_wm"),
      bienThe("Trắng", "L", "sku_wl"),
      bienThe("Xanh navy", "M", "sku_nm"),
      bienThe("Xanh navy", "L", "sku_nl"),
    ],
    [
      offer("sku_wm", true),
      offer("sku_wl", true),
      offer("sku_nm", true),
      offer("sku_nl", true),
    ],
  );
  const xanh = mau.find((m) => m.mau === "Xanh navy")!;

  // Khách đang xem Trắng/L, bấm sang Xanh navy → phải ra Xanh navy/L.
  expect(doiMau(xanh, "L")).toBe("sku_nl");
});

test("đổi sang màu KHÔNG có size đang chọn thì lùi về size còn hàng", () => {
  const mau = gomTheoMau(
    [
      bienThe("Trắng", "XL", "sku_wxl"),
      bienThe("Xanh navy", "M", "sku_nm"),
      bienThe("Xanh navy", "L", "sku_nl"),
    ],
    [
      offer("sku_wxl", true),
      // M hết, L còn.
      offer("sku_nm", false),
      offer("sku_nl", true),
    ],
  );
  const xanh = mau.find((m) => m.mau === "Xanh navy")!;

  expect(doiMau(xanh, "XL")).toBe("sku_nl");
});

test("không có biến thể nào thì không nổ", () => {
  const mau: MotMau[] = gomTheoMau(undefined, []);
  expect(mau).toEqual([]);
  expect(chonBanDau(mau)).toBeNull();
});
