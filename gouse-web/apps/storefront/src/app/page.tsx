"use client";

import {
  isApiError,
  listBuyBoxPrices,
  listProducts,
  type BuyBoxPrice,
  type NhomMau,
  type ProductList,
} from "@fc/api-client";
import { Alert } from "@fc/ui";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import * as React from "react";

import { BoLocMau } from "@/components/bo-loc-mau";
import { money } from "@/lib/format";
import { docMauTuURL, NHOM_MAU } from "@/lib/mau";
import { useShop } from "@/lib/shop";

/**
 * Trang chủ — danh sách sản phẩm.
 *
 * # Giá tra RIÊNG, không nằm trong danh mục
 *
 * Giá thuộc về OFFER, và module `product` cùng tầng với `marketplace` nên
 * không gọi được. Nhồi giá vào danh mục sẽ bắt mọi lời gọi sản phẩm kéo
 * theo truy vấn giá, kể cả trang quản trị nơi không hiển thị giá bán.
 *
 * Trang gọi thêm MỘT lượt cho cả danh sách — cùng mẫu với việc tra tên
 * nhà bán ở trang chi tiết.
 *
 * # `price_from`, không phải "giá"
 *
 * Một sản phẩm có thể có nhiều nhà bán với giá khác nhau. Con số ở đây là
 * giá THẤP NHẤT trong các offer ĐANG THẮNG BUY BOX — tức giá khách thật sự
 * mua được, đã loại offer hết hàng. Giá thật khách trả phụ thuộc offer họ
 * chọn ở trang chi tiết.
 *
 * # Trước 26/08 chỗ này hiện dấu gạch
 *
 * Đặc tả khai `price_from` là bắt buộc trên `ProductSummary` trong khi API
 * chưa bao giờ trả nó. TypeScript tin đặc tả nên không báo gì, và cửa hàng
 * liệt kê sản phẩm không có giá suốt nhiều tuần.
 *
 * # Bộ lọc nằm ở URL
 *
 * `?color=BLACK,WHITE` chứ không phải `useState`. Ba thứ chỉ có khi trạng
 * thái ở URL: gửi link cho bạn bè kèm đúng bộ lọc, bấm Back quay về lựa
 * chọn trước, và tải lại trang không mất gì.
 *
 * `useSearchParams` buộc phải nằm trong một ranh giới `<Suspense>`, nếu
 * không cả cây component phía trên nó mất khả năng render sẵn — xem
 * `node_modules/next/dist/docs/01-app/.../use-search-params.md`.
 */
export default function HomePage() {
  return (
    <React.Suspense fallback={<p className="muted">Đang tải…</p>}>
      <DanhMuc />
    </React.Suspense>
  );
}

function DanhMuc() {
  const { api } = useShop();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [data, setData] = React.useState<ProductList | null>(null);

  const mau = React.useMemo(
    () => docMauTuURL(searchParams.get("color")),
    [searchParams],
  );

  // Ghép lại thành chuỗi để làm dependency: `mau` là mảng mới mỗi lần
  // render, nên dùng thẳng nó sẽ gọi lại API vô hạn.
  const khoaMau = mau.join(",");

  function doiMau(moi: NhomMau[]) {
    const q = new URLSearchParams(searchParams.toString());
    if (moi.length > 0) q.set("color", moi.join(","));
    else q.delete("color");

    // `replace` chứ không `push`: bật tắt năm màu liên tiếp sẽ nhồi năm
    // mục vào lịch sử, và khách bấm Back năm lần mới rời được trang.
    //
    // `scroll: false`: danh sách đổi ngay dưới bộ lọc, nhảy lên đầu trang
    // làm mất chỗ khách đang nhìn.
    router.replace(q.size > 0 ? `${pathname}?${q}` : pathname, { scroll: false });
  }

  // Giá tra RIÊNG: danh mục cố ý không chứa giá.
  //
  // Một lượt gọi cho cả danh sách, và gọi SAU khi đã hiện sản phẩm — tên
  // và ảnh không phải chờ giá. Sản phẩm không có offer bán được thì vắng
  // mặt ở đây, và thẻ hiện dấu gạch thay vì một con số sai.
  const [gia, setGia] = React.useState<Record<string, BuyBoxPrice>>({});
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await listProducts(api, {
          limit: 24,
          color: khoaMau === "" ? [] : (khoaMau.split(",") as NhomMau[]),
        });
        if (cancelled) return;
        setData(res);

        const ids = (res.data ?? []).map((p) => p.id);
        const prices = await listBuyBoxPrices(api, ids);
        if (cancelled) return;
        setGia(Object.fromEntries(prices.map((x) => [x.product_id, x])));
      } catch (e) {
        if (!cancelled) {
          setError(isApiError(e) ? e.message : "Không tải được danh sách sản phẩm");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api, khoaMau]);

  if (error) return <Alert tone="danger">{error}</Alert>;

  const products = data?.data ?? [];

  return (
    <div>
      <h1>Sản phẩm</h1>

      <BoLocMau dangChon={mau} onDoi={doiMau} />

      {/*
        Trạng thái rỗng phải nói ĐÚNG lý do.
        "Chưa có sản phẩm nào được đăng bán" trong lúc khách vừa lọc màu
        tím là một câu sai — và nó khiến họ nghĩ cửa hàng trống, chứ không
        nghĩ tới việc bỏ bộ lọc.
      */}
      {!data && <p className="muted">Đang tải…</p>}

      {data && products.length === 0 && (
        <p className="muted">
          {mau.length > 0 ? (
            <>
              Không có sản phẩm nào màu{" "}
              <strong>{mau.map((m) => NHOM_MAU[m].nhan).join(", ")}</strong>.{" "}
              <button type="button" className="lien-ket" onClick={() => doiMau([])}>
                Bỏ lọc màu
              </button>
            </>
          ) : (
            "Chưa có sản phẩm nào được đăng bán."
          )}
        </p>
      )}

      <ul className="grid">
        {products.map((p) => (
          <li key={p.id} className="card">
            <Link href={`/products/${p.id}`} className="card__link">
              <div className="card__media">
                {p.primary_image_url ? (
                  // Ảnh từ CDN của nhà bán: dùng <img> thay vì next/image
                  // để không phải khai báo trước mọi tên miền có thể có.
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={p.primary_image_url} alt="" loading="lazy" />
                ) : (
                  <div className="card__placeholder" aria-hidden="true" />
                )}
              </div>

              <div className="card__body">
                <p className="card__brand">{p.brand?.name}</p>
                <h2 className="card__name">{p.name}</h2>
                <p className="card__price">
                  {gia[p.id] ? money(gia[p.id]!.price_from) : "—"}
                  {gia[p.id]?.compare_at_price && (
                    <span className="card__compare">
                      {money(gia[p.id]!.compare_at_price)}
                    </span>
                  )}
                </p>
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
