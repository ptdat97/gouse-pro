import type { ProductDetail, ProductOffers } from "@fc/api-client";

type Offer = NonNullable<ProductOffers["data"]>[number];
type Variant = NonNullable<ProductDetail["variants"]>[number];

/** MotSize là một size chọn được, kèm mã SKU đứng sau nó. */
export type MotSize = {
  size: string;
  skuId: string;

  /** coHang: có ít nhất một offer ĐANG BÁN cho SKU này. */
  coHang: boolean;
};

/** MotMau gom mọi size của cùng một màu. */
export type MotMau = {
  mau: string;
  anh?: string;
  sizes: MotSize[];
  coHang: boolean;
};

/**
 * gomTheoMau dựng cây chọn hàng từ danh sách biến thể phẳng.
 *
 * # Vì sao phải gom ở đây
 *
 * Ở domain, `Variant` là MỘT TỔ HỢP THUỘC TÍNH đầy đủ — `{màu: Trắng,
 * size: M}` — chứ không phải "màu Trắng có các size". Dùng map thuộc tính
 * là quyết định có chủ ý: áo có độ dài tay, quần có kiểu ống, giày không
 * có cả hai, nên trường cố định sẽ chật.
 *
 * Hệ quả là API trả một danh sách PHẲNG, trong đó "Trắng" xuất hiện nhiều
 * lần — mỗi lần một size. Khách thì không mua theo cách đó: họ chọn màu
 * trước, rồi mới chọn size trong màu ấy. Việc gom là của TRANG.
 *
 * # Vì sao "còn hàng" hỏi OFFER chứ không hỏi tồn kho
 *
 * Trong một cái chợ, mua được hay không là câu hỏi về LỜI CHÀO BÁN, không
 * phải về số hàng trong kho. SKU còn 100 cái mà không nhà bán nào đang
 * chào bán thì khách vẫn không mua được — và ngược lại, tồn kho là dữ liệu
 * của nền tảng, không phải thứ trang sản phẩm nên tự diễn giải.
 *
 * `is_sellable` là câu trả lời máy chủ đã tổng hợp sẵn (tồn kho + trạng
 * thái nhà bán + trạng thái offer). Suy lại ở đây là cài quy tắc lần thứ
 * hai, và hai bản sẽ lệch.
 */
export function gomTheoMau(
  variants: Variant[] | undefined,
  offers: Offer[],
): MotMau[] {
  const banDuoc = new Set(
    offers.filter((o) => o.is_sellable).map((o) => o.sku_id),
  );

  const theoMau = new Map<string, MotMau>();

  for (const v of variants ?? []) {
    const mau = v.color;
    let nhom = theoMau.get(mau);
    if (!nhom) {
      nhom = { mau, anh: anhCuaBienThe(v), sizes: [], coHang: false };
      theoMau.set(mau, nhom);
    }
    // Ảnh lấy từ biến thể ĐẦU TIÊN của màu có ảnh. Ảnh là thuộc tính của
    // MÀU, không phải của size — áo trắng size M và size L là cùng một
    // tấm ảnh.
    nhom.anh ??= anhCuaBienThe(v);

    for (const sku of v.skus ?? []) {
      // Size trùng trong cùng một màu: giữ bản ghi ĐẦU TIÊN. Hai SKU cùng
      // (màu, size) là dữ liệu mâu thuẫn; đoán bản nào đúng còn tệ hơn
      // chọn ổn định một bản.
      if (nhom.sizes.some((s) => s.size === sku.size)) continue;

      nhom.sizes.push({
        size: sku.size,
        skuId: sku.id,
        coHang: banDuoc.has(sku.id),
      });
    }
  }

  const out = [...theoMau.values()];
  for (const nhom of out) {
    nhom.coHang = nhom.sizes.some((s) => s.coHang);
  }
  return out;
}

function anhCuaBienThe(v: Variant): string | undefined {
  return (v.images ?? [])[0]?.url;
}

/**
 * chonBanDau chọn sẵn màu và size khi trang vừa mở.
 *
 * Ưu tiên tổ hợp MUA ĐƯỢC: mở trang ra mà lựa chọn mặc định đã hết hàng
 * là bắt khách làm thêm một bước chỉ để về lại trạng thái dùng được.
 *
 * Không có tổ hợp nào mua được thì vẫn chọn cái đầu tiên, KHÔNG để trống:
 * trang phải nói rõ "hết hàng" chứ không phải trông như chưa tải xong.
 */
export function chonBanDau(
  mauList: MotMau[],
): { mau: string; skuId: string } | null {
  const uuTien = mauList.find((m) => m.coHang) ?? mauList[0];
  if (!uuTien) return null;

  const size = uuTien.sizes.find((s) => s.coHang) ?? uuTien.sizes[0];
  if (!size) return null;

  return { mau: uuTien.mau, skuId: size.skuId };
}

/**
 * doiMau trả SKU nên chọn khi khách đổi sang màu khác.
 *
 * GIỮ NGUYÊN SIZE nếu màu mới có size đó. Khách nghĩ "tôi mặc M, cho tôi
 * xem M màu xanh" — bắt họ chọn lại size mỗi lần đổi màu là hiểu sai việc
 * họ đang làm. Đây cũng là chỗ dễ sinh lỗi im lặng: giữ nguyên `skuId` cũ
 * thì khách tưởng đang xem màu xanh nhưng lại mua đúng chiếc áo trắng.
 */
export function doiMau(mauMoi: MotMau, sizeDangChon: string | null): string | null {
  const giuSize = mauMoi.sizes.find((s) => s.size === sizeDangChon);
  if (giuSize) return giuSize.skuId;

  const thayThe = mauMoi.sizes.find((s) => s.coHang) ?? mauMoi.sizes[0];
  return thayThe?.skuId ?? null;
}
