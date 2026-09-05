"use client";

import {
  completeCheckout,
  isApiError,
  setCheckoutShippingAddress,
  setCheckoutShippingMethod,
  startCheckout,
  type Checkout,
} from "@fc/api-client";
import { Alert, Button, Field, Input, Select } from "@fc/ui";
import Link from "next/link";
import { useRouter } from "next/navigation";
import * as React from "react";

import { countdown, money, msConLai } from "@/lib/format";
import { useShop } from "@/lib/shop";

/**
 * Trang thanh toán.
 *
 * # Phiên mở LÚC VÀO TRANG, không phải lúc bấm "Đặt hàng"
 *
 * Mở phiên là GIỮ TỒN KHO — từ đó khách có 15 phút và giá được đóng băng.
 * Mở sớm hơn (ngay ở trang giỏ) là khóa hàng của người khác trong khi khách
 * còn đang lưỡng lự; mở muộn hơn (lúc bấm đặt) thì khách điền xong địa chỉ
 * mới biết hết hàng.
 *
 * # Đồng hồ đếm ngược là BẮT BUỘC hiển thị
 *
 * Hết hạn thì hàng nhả về kho và phiên không dùng được nữa. Không báo trước
 * nghĩa là khách điền xong form rồi nhận lỗi không hiểu vì sao.
 *
 * # Khóa idempotency gắn với PHIÊN
 *
 * Xem `completeCheckout` ở @fc/api-client: bấm hai lần hoặc client tự gửi
 * lại không được tạo hai đơn.
 */
export default function CheckoutPage() {
  const { api, cart, cartLoading, refreshCart, me } = useShop();
  const router = useRouter();

  const [checkout, setCheckout] = React.useState<Checkout | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [now, setNow] = React.useState(() => Date.now());

  // Liên hệ của khách VÃNG LAI.
  //
  // Backend từ chối mở phiên khi không có danh tính người mua: không có
  // email thì không gửi được xác nhận đơn, và khách không có gì trong tay
  // để biết đơn đã đặt xong.
  //
  // Số điện thoại KHÔNG phải tùy chọn cho có: trang tra cứu đơn nhận mã
  // đơn + SỐ ĐIỆN THOẠI. Khách vãng lai không để lại số thì sau này không
  // có đường nào xem lại đơn của chính mình.
  const [lienHe, setLienHe] = React.useState({ email: "", phone: "" });
  const laVangLai = !me;

  const cartId = cart?.cart?.id;

  // Mở phiên MỘT lần cho mỗi giỏ. `started` chặn việc React chạy effect
  // hai lần ở chế độ nghiêm ngặt tạo hai phiên — mà hai phiên nghĩa là
  // hàng bị giữ gấp đôi.
  const started = React.useRef<string | null>(null);

  // Mở phiên cho khách ĐÃ ĐĂNG NHẬP ngay khi vào trang.
  //
  // Khách vãng lai thì CHỜ họ điền liên hệ đã: mở phiên là giữ tồn kho 15
  // phút, và giữ hàng cho một người chưa chắc mua được là khóa hàng của
  // người khác vô ích.
  React.useEffect(() => {
    if (!cartId || laVangLai || started.current === cartId) return;
    started.current = cartId;
    void moPhien();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, cartId, laVangLai]);

  async function moPhien(guest?: { email: string; phone: string }) {
    setBusy(true);
    setError(null);
    try {
      setCheckout(await startCheckout(api, cartId!, guest));
    } catch (e) {
      // Mở phiên hỏng thì cho thử LẠI: xóa dấu để lần bấm sau chạy tiếp.
      started.current = null;
      setError(isApiError(e) ? e.message : "Không mở được phiên thanh toán");
    } finally {
      setBusy(false);
    }
  }

  // Nhịp đồng hồ đếm ngược.
  React.useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);

  if (cartLoading && !cart) return <p className="muted">Đang tải…</p>;

  if (!cartId || (cart?.cart?.groups?.length ?? 0) === 0) {
    return (
      <div>
        <h1>Thanh toán</h1>
        <p className="muted">Giỏ hàng trống — chưa có gì để thanh toán.</p>
      </div>
    );
  }

  if (error && !checkout) return <Alert tone="danger">{error}</Alert>;
  // Khách VÃNG LAI: hỏi liên hệ TRƯỚC khi giữ hàng.
  //
  // Đây là bước duy nhất trong luồng mua hàng bắt khách nhập gì đó trước
  // khi thấy tiến triển, và nó đáng: mở phiên là khóa tồn kho 15 phút.
  if (!checkout && laVangLai) {
    return (
      <div>
        <h1>Thanh toán</h1>
        {error && <Alert tone="danger">{error}</Alert>}

        <section className="panel">
          <h2>Thông tin liên hệ</h2>
          <p className="muted">
            Chúng tôi gửi xác nhận đơn qua email. Số điện thoại dùng để bạn{" "}
            <strong>tra cứu đơn</strong> về sau — không có nó thì sau này
            không xem lại được đơn của mình.
          </p>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              void moPhien(lienHe);
            }}
          >
            <Field label="Email" htmlFor="guest_email">
              <Input
                id="guest_email"
                name="guest_email"
                type="email"
                required
                value={lienHe.email}
                onChange={(e) =>
                  setLienHe((v) => ({ ...v, email: e.target.value }))
                }
              />
            </Field>

            <Field label="Số điện thoại" htmlFor="guest_phone">
              <Input
                id="guest_phone"
                name="guest_phone"
                required
                value={lienHe.phone}
                onChange={(e) =>
                  setLienHe((v) => ({ ...v, phone: e.target.value }))
                }
              />
            </Field>

            <div className="actions">
              <Button type="submit" variant="primary" disabled={busy}>
                {busy ? "Đang giữ hàng…" : "Tiếp tục"}
              </Button>
            </div>
          </form>

          <p className="muted">
            Đã có tài khoản? <Link href="/dang-nhap">Đăng nhập</Link> để không
            phải nhập lại.
          </p>
        </section>
      </div>
    );
  }

  if (!checkout) return <p className="muted">Đang giữ hàng cho bạn…</p>;

  // Dùng CHUNG nguồn với đồng hồ đếm ngược.
  //
  // Trước đây trang tự tính, và khi `expires_at` hỏng thì `msLeft` là NaN:
  // `NaN <= 0` là false nên trang coi phiên CHƯA hết hạn và cho bấm tiếp,
  // trong khi đồng hồ hiện "00:00". Hai chỗ cùng suy ra một sự thật rồi
  // bất đồng.
  //
  // `null` nghĩa là KHÔNG BIẾT — không coi là đã hết hạn, vì chặn khách
  // vì một mốc thời gian không đọc được thì tệ hơn để họ thử.
  void now; // `now` chỉ để buộc render lại mỗi giây
  const msLeft = msConLai(checkout.expires_at);
  const expired = msLeft !== null && msLeft <= 0;

  async function step(fn: () => Promise<Checkout>) {
    setBusy(true);
    setError(null);
    try {
      setCheckout(await fn());
    } catch (e) {
      setError(isApiError(e) ? e.message : "Thao tác không thành công");
    } finally {
      setBusy(false);
    }
  }

  async function onSubmitAddress(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    await step(() =>
      setCheckoutShippingAddress(api, checkout!.id, {
        recipient_name: String(f.get("recipient_name") ?? ""),
        phone: String(f.get("phone") ?? ""),
        street_address: String(f.get("street_address") ?? ""),
        ward: String(f.get("ward") ?? ""),
        district: String(f.get("district") ?? ""),
        province: String(f.get("province") ?? ""),
        country_code: "VN",
      }),
    );
  }

  async function onPlaceOrder() {
    setBusy(true);
    setError(null);
    try {
      const res = await completeCheckout(api, checkout!.id, "COD");
      // Làm mới giỏ TRƯỚC khi rời trang: giỏ đã thành đơn, để lại số cũ
      // trên biểu tượng là nói dối khách.
      await refreshCart();
      // Kèm số điện thoại cho khách VÃNG LAI.
      //
      // Trang chi tiết đơn nhận danh tính qua header `X-Guest-Phone`, lấy
      // từ query — cùng đường mà trang tra cứu đơn dùng. Thiếu nó thì
      // khách vừa đặt hàng xong đã thấy ngay "Không tìm thấy đơn hàng",
      // và đó là kết thúc tệ nhất có thể cho đường đi của tiền.
      //
      // Khách đã đăng nhập không cần: token của họ đã nói họ là ai.
      const q = new URLSearchParams({ placed: "1" });
      if (laVangLai && lienHe.phone) {
        q.set("phone", lienHe.phone);
      }
      router.push(
        `/orders/${res.order?.order_number ?? ""}?${q.toString()}`,
      );
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không đặt được đơn hàng");
      setBusy(false);
    }
  }

  const hasAddress = Boolean(checkout.shipping_address?.recipient_name);
  const hasShipping = (checkout.shipping_fee?.amount ?? 0) > 0;

  return (
    <div>
      <h1>Thanh toán</h1>

      <p>
        Hàng được giữ cho bạn trong{" "}
        <span
          className={`countdown${msLeft !== null && msLeft < 120_000 ? " countdown--urgent" : ""}`}
        >
          {countdown(checkout.expires_at)}
        </span>
      </p>

      {expired && (
        <Alert tone="danger">
          Phiên thanh toán đã hết hạn và hàng đã được trả về kho. Vui lòng
          quay lại giỏ hàng và bắt đầu lại.
        </Alert>
      )}
      {error && <Alert tone="danger">{error}</Alert>}

      <div className="steps">
        <div>
          <section className="panel">
            <h2>1. Địa chỉ giao hàng</h2>
            <form onSubmit={onSubmitAddress} className="stack">
              <div className="form-row form-row--2">
                <Field label="Người nhận" htmlFor="recipient_name">
                  <Input id="recipient_name" name="recipient_name" required
                    defaultValue={checkout.shipping_address?.recipient_name} />
                </Field>
                <Field label="Số điện thoại" htmlFor="phone">
                  <Input id="phone" name="phone" required
                    defaultValue={checkout.shipping_address?.phone} />
                </Field>
              </div>

              <Field label="Địa chỉ" htmlFor="street_address">
                <Input id="street_address" name="street_address" required
                  defaultValue={checkout.shipping_address?.street_address} />
              </Field>

              <div className="form-row form-row--2">
                <Field label="Phường/Xã" htmlFor="ward">
                  <Input id="ward" name="ward"
                    defaultValue={checkout.shipping_address?.ward} />
                </Field>
                <Field label="Quận/Huyện" htmlFor="district">
                  <Input id="district" name="district"
                    defaultValue={checkout.shipping_address?.district} />
                </Field>
              </div>

              <Field label="Tỉnh/Thành phố" htmlFor="province">
                <Input id="province" name="province" required
                  defaultValue={checkout.shipping_address?.province} />
              </Field>

              <div className="actions">
                <Button type="submit" disabled={busy || expired}>
                  Lưu địa chỉ
                </Button>
              </div>
            </form>
          </section>

          <section className="panel">
            <h2>2. Phương thức vận chuyển</h2>
            <p className="muted">
              Phí vận chuyển do hệ thống tính — bạn chỉ chọn hình thức.
            </p>
            <Field label="Hình thức" htmlFor="shipping_method">
              <Select
                id="shipping_method"
                disabled={busy || expired || !hasAddress}
                defaultValue=""
                onChange={(e) => {
                  const v = e.target.value;
                  if (v === "STANDARD" || v === "EXPRESS") {
                    void step(() =>
                      setCheckoutShippingMethod(api, checkout.id, v),
                    );
                  }
                }}
              >
                <option value="" disabled>
                  Chọn hình thức giao hàng
                </option>
                <option value="STANDARD">Tiêu chuẩn</option>
                <option value="EXPRESS">Nhanh</option>
              </Select>
            </Field>
            {!hasAddress && (
              <p className="muted">Nhập địa chỉ trước để chọn được hình thức.</p>
            )}
          </section>

          <section className="panel">
            <h2>3. Thanh toán</h2>
            <p className="muted">
              Hiện chỉ hỗ trợ thanh toán khi nhận hàng (COD).
            </p>
            <div className="actions">
              <Button
                variant="primary"
                onClick={onPlaceOrder}
                disabled={busy || expired || !hasAddress || !hasShipping}
              >
                {busy ? "Đang đặt hàng…" : "Đặt hàng"}
              </Button>
            </div>
            {!hasShipping && hasAddress && (
              <p className="muted">Chọn hình thức giao hàng để đặt đơn.</p>
            )}
          </section>
        </div>

        <aside className="panel">
          <h2>Đơn hàng</h2>
          <ul className="lines">
            {(checkout.lines ?? []).map((l, i) => (
              <li key={i} className="line">
                <div>
                  <strong>{l.product_name}</strong>
                  {l.variant_description && (
                    <div className="muted">{l.variant_description}</div>
                  )}
                  <div className="muted">SL {l.quantity}</div>
                </div>
                <div>{money(l.line_total)}</div>
              </li>
            ))}
          </ul>

          <div className="totals">
            <div className="totals__row">
              <span>Tạm tính</span>
              <span>{money(checkout.subtotal)}</span>
            </div>
            <div className="totals__row">
              <span>Phí vận chuyển</span>
              <span>{money(checkout.shipping_fee)}</span>
            </div>
            {(checkout.discount_amount?.amount ?? 0) > 0 && (
              <div className="totals__row">
                <span>Giảm giá</span>
                <span>−{money(checkout.discount_amount)}</span>
              </div>
            )}
            <div className="totals__row totals__row--grand">
              <span>Tổng cộng</span>
              <span>{money(checkout.total)}</span>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
