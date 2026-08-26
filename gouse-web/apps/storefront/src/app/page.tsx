"use client";

import {
  isApiError,
  listBuyBoxPrices,
  listProducts,
  type BuyBoxPrice,
  type ProductList,
} from "@fc/api-client";
import { Alert } from "@fc/ui";
import Link from "next/link";
import * as React from "react";

import { money } from "@/lib/format";
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
 */
export default function HomePage() {
  const { api } = useShop();
  const [data, setData] = React.useState<ProductList | null>(null);

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
        const res = await listProducts(api, { limit: 24 });
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
  }, [api]);

  if (error) return <Alert tone="danger">{error}</Alert>;
  if (!data) return <p className="muted">Đang tải…</p>;

  const products = data.data ?? [];
  if (products.length === 0) {
    return (
      <div>
        <h1>Sản phẩm</h1>
        <p className="muted">Chưa có sản phẩm nào được đăng bán.</p>
      </div>
    );
  }

  return (
    <div>
      <h1>Sản phẩm</h1>

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
