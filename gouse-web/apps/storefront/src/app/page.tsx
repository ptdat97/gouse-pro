"use client";

import { isApiError, listProducts, type ProductList } from "@fc/api-client";
import { Alert } from "@fc/ui";
import Link from "next/link";
import * as React from "react";

import { money } from "@/lib/format";
import { useShop } from "@/lib/shop";

/**
 * Trang chủ — danh sách sản phẩm.
 *
 * # `price_from`, không phải "giá"
 *
 * Một sản phẩm có thể có nhiều nhà bán với giá khác nhau. Con số ở đây là
 * giá THẤP NHẤT; giá thật khách trả phụ thuộc offer họ chọn ở trang chi
 * tiết. Hiển thị nó như "giá" là hứa một con số có thể không mua được.
 */
export default function HomePage() {
  const { api } = useShop();
  const [data, setData] = React.useState<ProductList | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await listProducts(api, { limit: 24 });
        if (!cancelled) setData(res);
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
                  {money(p.price_from)}
                  {p.compare_at_price && (
                    <span className="card__compare">
                      {money(p.compare_at_price)}
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
