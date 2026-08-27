"use client";

import {
  getProduct,
  isApiError,
  listProductOffers,
  listSellersByIds,
  type ProductDetail,
  type ProductOffers,
  type SellerRef,
} from "@fc/api-client";
import { Alert, Button, Input } from "@fc/ui";
import { useRouter } from "next/navigation";
import * as React from "react";

import { chonBanDau, doiMau, gomTheoMau } from "@/lib/chon-hang";
import { money } from "@/lib/format";
import { useShop } from "@/lib/shop";

type Offer = NonNullable<ProductOffers["data"]>[number];

/**
 * Chi tiết sản phẩm — nơi khách quyết định mua gì.
 *
 * # Ba lựa chọn, theo đúng thứ tự khách nghĩ
 *
 *   màu → size → nhà bán
 *
 * Không phải thứ tự tùy ý. Màu và size xác định MÓN HÀNG (một SKU); nhà
 * bán chỉ là chọn mua món đó của ai. Đảo thứ tự — bắt chọn nhà bán trước —
 * là hỏi "mua của ai" khi còn chưa biết "mua cái gì".
 *
 * Bản trước của trang này bỏ hẳn hai bước đầu: nó đổ toàn bộ offer của mọi
 * SKU vào một danh sách. Khách chọn theo giá mà không biết mình đang mua
 * size nào — với thời trang thì đó không phải chi tiết phụ. Ba dòng cùng
 * gắn nhãn "Đề xuất" cũng vì vậy: buy box tính theo TỪNG SKU, nên gộp ba
 * SKU lại thì cả ba cùng thắng.
 *
 * # Hết hàng thì HIỆN, không ẩn
 *
 * Size và màu không mua được vẫn nằm nguyên trong danh sách, chỉ bị vô
 * hiệu hóa. Ẩn đi thì khách kết luận thương hiệu không làm size của mình
 * và rời đi — thay vì biết là tạm hết.
 *
 * # Việc gom dữ liệu nằm ở lib/chon-hang.ts
 *
 * Đó là logic thuần, có test riêng chạy không cần trình duyệt. Ở đây chỉ
 * còn việc hiển thị và nối sự kiện.
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
  const [sellers, setSellers] = React.useState<Record<string, SellerRef>>({});

  const [skuID, setSkuID] = React.useState<string | null>(null);
  const [mauDangChon, setMauDangChon] = React.useState<string | null>(null);
  const [offerID, setOfferID] = React.useState<string | null>(null);

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

        const list = o.data ?? [];
        setProduct(p);
        setOffers(list);

        const banDau = chonBanDau(gomTheoMau(p.variants, list));
        if (banDau) {
          setMauDangChon(banDau.mau);
          setSkuID(banDau.skuId);
        }

        // Tên nhà bán tra SAU, không nằm trong Promise.all: giá và nút mua
        // không được chờ một thông tin chỉ dùng để hiển thị.
        const ids = [...new Set(list.map((x) => x.seller_id))];
        const found = await listSellersByIds(api, ids);
        if (cancelled) return;
        setSellers(Object.fromEntries(found.map((sl) => [sl.id, sl])));
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

  const mauList = React.useMemo(
    () => gomTheoMau(product?.variants, offers),
    [product, offers],
  );

  const mau = mauList.find((m) => m.mau === mauDangChon) ?? mauList[0];
  const sizeDangChon =
    mau?.sizes.find((s) => s.skuId === skuID)?.size ?? null;

  // Offer của ĐÚNG món hàng đang chọn. Đây là điều làm nhãn "Đề xuất" có
  // nghĩa trở lại: trong phạm vi một SKU, buy box chỉ có một người thắng.
  const offerCuaSku = React.useMemo(
    () => offers.filter((o) => o.sku_id === skuID),
    [offers, skuID],
  );

  // Chọn sẵn offer thắng buy box mỗi khi món hàng đổi.
  React.useEffect(() => {
    const banDuoc = offerCuaSku.filter((o) => o.is_sellable);
    const best =
      banDuoc.find((o) => o.is_buy_box) ?? banDuoc[0] ?? offerCuaSku[0];
    setOfferID(best?.id ?? null);
  }, [offerCuaSku]);

  if (error) return <Alert tone="danger">{error}</Alert>;
  if (!product) return <p className="muted">Đang tải…</p>;

  const p = product;
  const chosen = offerCuaSku.find((o) => o.id === offerID);
  const canBuy = chosen?.is_sellable === true;

  // Ảnh theo MÀU đang chọn. Khách chọn màu xanh phải thấy áo xanh — ảnh
  // sai màu là một nguyên nhân hoàn hàng.
  const anhSanPham = [...(p.images ?? [])].sort(
    (a, b) => (a.order ?? 0) - (b.order ?? 0),
  )[0]?.url;
  const anh = mau?.anh ?? anhSanPham;

  function chonMau(ten: string) {
    const moi = mauList.find((m) => m.mau === ten);
    if (!moi) return;
    setMauDangChon(ten);
    setSkuID(doiMau(moi, sizeDangChon));
  }

  async function onAdd() {
    if (!offerID) return;
    setAdding(true);
    setError(null);
    try {
      await addItem(offerID, quantity);
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
        {anh ? <img src={anh} alt={p.name} /> : null}
      </div>

      <div>
        <h1>{p.name}</h1>
        {p.description ? <p className="muted">{p.description}</p> : null}

        {mauList.length === 0 ? (
          <Alert tone="warning">Sản phẩm này chưa có phiên bản nào để bán.</Alert>
        ) : (
          <>
            <ChonMau
              mauList={mauList}
              dangChon={mau?.mau ?? null}
              onChon={chonMau}
            />
            <ChonSize
              mau={mau}
              skuDangChon={skuID}
              onChon={setSkuID}
            />
          </>
        )}

        <ChonNhaBan
          offers={offerCuaSku}
          sellers={sellers}
          dangChon={offerID}
          onChon={setOfferID}
        />

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
          <Button onClick={onAdd} disabled={!offerID || !canBuy || adding}>
            {adding ? "Đang thêm…" : "Thêm vào giỏ"}
          </Button>
        </div>

        {mauList.length > 0 && offerCuaSku.length === 0 && (
          <p className="muted">
            Chưa có nhà bán nào chào bán phiên bản này.
          </p>
        )}
        {chosen && !canBuy && (
          <p className="muted">
            Nhà bán này đang hết hàng. Chọn nhà bán khác nếu có.
          </p>
        )}
      </div>
    </div>
  );
}

/**
 * ChonMau — bước 1.
 *
 * Màu hết sạch hàng vẫn hiện, chỉ mờ đi và ghi rõ. Bấm vào vẫn được: khách
 * có quyền xem ảnh màu đó rồi tự quyết định chờ hay đổi màu. Chặn hẳn là
 * giấu thông tin chứ không bảo vệ ai.
 */
function ChonMau({
  mauList,
  dangChon,
  onChon,
}: {
  mauList: ReturnType<typeof gomTheoMau>;
  dangChon: string | null;
  onChon: (mau: string) => void;
}) {
  if (mauList.length <= 1) return null;

  return (
    <section className="chon">
      <h2 className="chon__nhan">Màu</h2>
      <div role="radiogroup" aria-label="Màu">
        {mauList.map((m) => (
          <button
            key={m.mau}
            type="button"
            role="radio"
            aria-checked={m.mau === dangChon}
            className={`chip${m.mau === dangChon ? " chip--chon" : ""}${
              m.coHang ? "" : " chip--het"
            }`}
            onClick={() => onChon(m.mau)}
          >
            {m.mau}
            {!m.coHang && <span className="chip__ghi"> · hết hàng</span>}
          </button>
        ))}
      </div>
    </section>
  );
}

/**
 * ChonSize — bước 2.
 *
 * Size hết hàng bị VÔ HIỆU HÓA chứ không ẩn, và khác với màu: chọn một
 * size không mua được thì phần dưới trống rỗng, không nói thêm được gì.
 * Còn màu thì ít nhất đổi được ảnh.
 *
 * Ẩn size hết hàng là sai lầm phổ biến và tốn kém: khách kết luận thương
 * hiệu không làm size của họ và rời đi, thay vì biết là tạm hết.
 */
function ChonSize({
  mau,
  skuDangChon,
  onChon,
}: {
  mau: ReturnType<typeof gomTheoMau>[number] | undefined;
  skuDangChon: string | null;
  onChon: (skuId: string) => void;
}) {
  if (!mau || mau.sizes.length === 0) return null;

  return (
    <section className="chon">
      <h2 className="chon__nhan">Size</h2>
      <div role="radiogroup" aria-label="Size">
        {mau.sizes.map((s) => (
          <button
            key={s.skuId}
            type="button"
            role="radio"
            aria-checked={s.skuId === skuDangChon}
            disabled={!s.coHang}
            title={s.coHang ? undefined : "Tạm hết hàng"}
            className={`chip${s.skuId === skuDangChon ? " chip--chon" : ""}${
              s.coHang ? "" : " chip--het"
            }`}
            onClick={() => onChon(s.skuId)}
          >
            {s.size}
          </button>
        ))}
      </div>
    </section>
  );
}

/**
 * ChonNhaBan — bước 3.
 *
 * Chỉ hiện offer của ĐÚNG món hàng đã chọn ở hai bước trên. Nhờ vậy nhãn
 * "Đề xuất" có nghĩa trở lại: buy box tính theo từng SKU, nên trong phạm
 * vi này chỉ có một người thắng.
 *
 * Offer không bán được vẫn hiện (mờ đi): khách cần biết nhà bán đó CÓ bán
 * món này, chỉ là đang hết — đó là thông tin để quyết định chờ hay mua của
 * người khác.
 */
function ChonNhaBan({
  offers,
  sellers,
  dangChon,
  onChon,
}: {
  offers: Offer[];
  sellers: Record<string, SellerRef>;
  dangChon: string | null;
  onChon: (offerId: string) => void;
}) {
  if (offers.length === 0) return null;

  return (
    <section className="chon">
      <h2 className="chon__nhan">Nhà bán</h2>
      <div role="radiogroup" aria-label="Nhà bán">
        {offers.map((o) => (
          <label
            key={o.id}
            className={`offer${o.id === dangChon ? " offer--selected" : ""}${
              o.is_sellable ? "" : " offer--het"
            }`}
          >
            <div className="offer__row">
              <span>
                <input
                  type="radio"
                  name="offer"
                  value={o.id}
                  checked={o.id === dangChon}
                  onChange={() => onChon(o.id)}
                />{" "}
                <strong>{money(o.price)}</strong>
                {o.is_buy_box && o.is_sellable && " · Đề xuất"}
              </span>
              <span className="offer__seller">
                {o.condition === "NEW" ? "Hàng mới" : "Đã qua sử dụng"}
              </span>
            </div>

            <div className="offer__seller">
              {sellers[o.seller_id]?.name ?? "Đang tra nhà bán…"}
              {sellers[o.seller_id]?.is_official && " · Chính hãng"}
            </div>

            <div className="offer__seller">
              {o.is_sellable ? "Còn hàng" : "Hết hàng"}
              {o.handling_time_hours
                ? ` · Chuẩn bị ${o.handling_time_hours} giờ`
                : ""}
            </div>
          </label>
        ))}
      </div>
    </section>
  );
}
