"use client";

import {
  getProduct,
  isApiError,
  listProductOffers,
  type ProductDetail,
  type ProductOffers,
} from "@fc/api-client";
import { Alert, Button, Input } from "@fc/ui";
import { useRouter } from "next/navigation";
import * as React from "react";

import { money } from "@/lib/format";
import { useShop } from "@/lib/shop";

type Offer = NonNullable<ProductOffers["data"]>[number];

/**
 * Trang chi tiết sản phẩm.
 *
 * # Hai lời gọi, không phải một
 *
 * `getProduct` trả thứ ỔN ĐỊNH (tên, mô tả, ảnh); `listProductOffers` trả
 * thứ ĐỔI LIÊN TỤC (ai bán, giá bao nhiêu, còn hàng không). Gộp lại thì
 * hoặc phải bỏ cache cả trang mỗi lần một nhà bán đổi giá, hoặc hiện giá cũ.
 *
 * # Khách chọn OFFER, không chọn sản phẩm
 *
 * Nút "Thêm vào giỏ" gửi `offer_id`. Cùng một chiếc áo có thể do ba nhà bán
 * chào với ba giá và ba thời gian giao khác nhau — khách phải thấy mình
 * đang mua của ai.
 */
export default function ProductPage({
  params,
}: {
  params: Promise<{ productId: string }>;
}) {
  const { productId } = React.use(params);
  const { api, addItem } = useShop();
  const router = useRouter();

  const [product, setProduct] = React.useState<ProductDetail | null>(null);
  const [offers, setOffers] = React.useState<Offer[]>([]);
  const [selected, setSelected] = React.useState<string | null>(null);
  const [quantity, setQuantity] = React.useState(1);
  const [error, setError] = React.useState<string | null>(null);
  const [adding, setAdding] = React.useState(false);

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [p, o] = await Promise.all([
          getProduct(api, productId),
          listProductOffers(api, productId),
        ]);
        if (cancelled) return;
        setProduct(p);

        const list = o.data ?? [];
        setOffers(list);

        // Chọn sẵn offer thắng buy box: đó là lựa chọn nền tảng khuyến
        // nghị, và bắt khách tự chọn khi chỉ có một nhà bán là thừa.
        const best = list.find((x) => x.is_buy_box) ?? list[0];
        setSelected(best?.id ?? null);
      } catch (e) {
        if (!cancelled) {
          setError(isApiError(e) ? e.message : "Không tải được sản phẩm");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api, productId]);

  if (error) return <Alert tone="danger">{error}</Alert>;
  if (!product) return <p className="muted">Đang tải…</p>;

  const p = product;

  // Ảnh đầu tiên là ảnh chính. Đặc tả có `order` nhưng không hứa mảng đã
  // sắp xếp, nên sắp lại ở đây thay vì tin vào thứ tự trả về.
  const cover = [...(p.images ?? [])].sort(
    (a, b) => (a.order ?? 0) - (b.order ?? 0),
  )[0];

  const chosen = offers.find((o) => o.id === selected);

  // `is_sellable` false vẫn HIỆN offer — khách cần biết sản phẩm CÓ tổ hợp
  // màu/size đó, chỉ là đang không mua được. Ẩn đi thì họ tưởng nền tảng
  // không bán và bỏ đi.
  //
  // Dùng THẲNG `is_sellable`, không suy lại từ trường khác. Máy chủ đã tổng
  // hợp cả tồn kho lẫn trạng thái nhà bán vào cờ này; suy lại ở đây là cài
  // quy tắc lần thứ hai, và hai bản sẽ lệch.
  //
  // Trước đây chỗ này đọc `availability` — một trường đặc tả có khai nhưng
  // endpoint KHÔNG BAO GIỜ trả. Vì nó không `required`, TypeScript vẫn cho
  // qua, và hệ quả là nút "Thêm vào giỏ" khóa vĩnh viễn: cửa hàng không bán
  // được gì. Không log máy chủ nào ghi lại chuyện đó.
  const canBuy = chosen?.is_sellable === true;

  async function onAdd() {
    if (!selected) return;
    setAdding(true);
    setError(null);
    try {
      await addItem(selected, quantity);
      router.push("/cart");
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không thêm được vào giỏ");
    } finally {
      setAdding(false);
    }
  }

  return (
    <div className="product">
      <div className="product__media">
        {cover ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={cover.url} alt={cover.alt ?? p.name} />
        ) : null}
      </div>

      <div>
        <p className="muted">{p.brand?.name}</p>
        <h1>{p.name}</h1>
        {p.description && <p className="muted">{p.description}</p>}

        <h2>Chọn nhà bán</h2>
        {offers.length === 0 ? (
          <Alert tone="warning">
            Sản phẩm này hiện chưa có nhà bán nào chào bán.
          </Alert>
        ) : (
          <div role="radiogroup" aria-label="Nhà bán">
            {offers.map((o) => (
              <label
                key={o.id}
                className={`offer${o.id === selected ? " offer--selected" : ""}`}
              >
                <div className="offer__row">
                  <span>
                    <input
                      type="radio"
                      name="offer"
                      value={o.id}
                      checked={o.id === selected}
                      onChange={() => setSelected(o.id)}
                    />{" "}
                    <strong>{money(o.price)}</strong>
                    {o.is_buy_box && " · Đề xuất"}
                  </span>
                  <span className="offer__seller">
                    {o.condition === "NEW" ? "Hàng mới" : "Đã qua sử dụng"}
                  </span>
                </div>

                {/*
                  Chưa hiện TÊN nhà bán: chưa có endpoint công khai tra hồ
                  sơ nhà bán, và endpoint offer chỉ trả `seller_id`. Hiện ô
                  trống còn tệ hơn không hiện — khách tưởng giao diện hỏng.
                  Xem P3-19 trong backlog.
                */}
                <div className="offer__seller">
                  {o.is_sellable ? "Còn hàng" : "Hết hàng"}
                  {o.handling_time_hours
                    ? ` · Chuẩn bị ${o.handling_time_hours} giờ`
                    : ""}
                </div>
              </label>
            ))}
          </div>
        )}

        <div className="line__qty">
          <label htmlFor="qty">Số lượng</label>
          <Input
            id="qty"
            type="number"
            min={1}
            value={quantity}
            onChange={(e) => setQuantity(Math.max(1, Number(e.target.value)))}
          />
        </div>

        <div className="actions">
          <Button onClick={onAdd} disabled={!selected || !canBuy || adding}>
            {adding ? "Đang thêm…" : "Thêm vào giỏ"}
          </Button>
        </div>

        {chosen && !canBuy && (
          <p className="muted">
            Nhà bán này đang hết hàng. Chọn nhà bán khác nếu có.
          </p>
        )}
      </div>
    </div>
  );
}
