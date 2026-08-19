"use client";

import { Alert, Button, Input } from "@fc/ui";
import Link from "next/link";
import * as React from "react";

import { availabilityLabel, money } from "@/lib/format";
import { useShop } from "@/lib/shop";

/**
 * Trang giỏ hàng.
 *
 * # Nhóm theo NHÀ BÁN, không phải một danh sách phẳng
 *
 * Backend trả `groups` vì hàng từ hai nguồn sẽ về hai gói, giao hai thời
 * điểm. Gộp phẳng ở đây là giấu điều khách cần biết trước khi đặt.
 *
 * # Tổng tiền do BACKEND tính
 *
 * Trang này không cộng gì cả. Cộng lại ở đây nghĩa là hai nơi cùng tính
 * một con số, và khách sẽ thấy một số ở giỏ, số khác ở bước thanh toán.
 */
export default function CartPage() {
  const { cart, cartLoading, cartError, updateItem, removeItem, mergeWarnings } =
    useShop();

  if (cartLoading && !cart) return <p className="muted">Đang tải giỏ hàng…</p>;

  const c = cart?.cart;
  const groups = c?.groups ?? [];

  if (groups.length === 0) {
    return (
      <div>
        <h1>Giỏ hàng</h1>
        {cartError && <Alert tone="danger">{cartError}</Alert>}
        <p className="muted">Giỏ hàng của bạn đang trống.</p>
        <div className="actions">
          <Link href="/">
            <Button>Xem sản phẩm</Button>
          </Link>
        </div>
      </div>
    );
  }

  // Món không mua được KHÔNG chặn thanh toán: phiên chỉ nhận các món mua
  // được, và bắt khách tự xóa trước mới cho đi tiếp là thêm một bước thừa.
  const hasUnavailable = groups.some((g) =>
    (g.items ?? []).some((i) => i.availability !== "IN_STOCK"),
  );

  return (
    <div>
      <h1>Giỏ hàng</h1>
      {cartError && <Alert tone="danger">{cartError}</Alert>}

      {/* Cảnh báo gộp giỏ PHẢI hiển thị: bỏ qua im lặng nghĩa là khách
          đăng nhập xong thấy ít hàng hơn mà không hiểu vì sao. */}
      {mergeWarnings.length > 0 && (
        <Alert tone="warning">
          Khi gộp giỏ lúc đăng nhập, một số món không giữ được trọn vẹn:
          <ul>
            {mergeWarnings.map((w, i) => (
              <li key={i}>{w}</li>
            ))}
          </ul>
        </Alert>
      )}

      {hasUnavailable && (
        <Alert tone="warning">
          Một số món không còn mua được. Chúng vẫn nằm trong giỏ để bạn quyết
          định, nhưng sẽ không được tính vào đơn.
        </Alert>
      )}

      {groups.map((g) => (
        <section key={g.seller?.id ?? "?"} className="group">
          <p className="group__seller">
            Bán bởi {g.seller?.name || "Nhà bán"}
          </p>

          <ul className="lines">
            {(g.items ?? []).map((item) => (
              <li key={item.id} className="line">
                <div>
                  <strong>{item.product_name}</strong>
                  {item.variant_description && (
                    <div className="muted">{item.variant_description}</div>
                  )}

                  {item.availability !== "IN_STOCK" && (
                    <div className="muted">
                      {availabilityLabel(item.availability)}
                    </div>
                  )}

                  <div className="line__qty">
                    <label htmlFor={`qty-${item.id}`} className="muted">
                      SL
                    </label>
                    <Input
                      id={`qty-${item.id}`}
                      type="number"
                      min={1}
                      defaultValue={item.quantity}
                      onBlur={(e) => {
                        const n = Math.max(1, Number(e.target.value));
                        if (n !== item.quantity) {
                          void updateItem(item.id, n);
                        }
                      }}
                    />
                    <Button
                      variant="secondary"
                      onClick={() => void removeItem(item.id)}
                    >
                      Xóa
                    </Button>
                  </div>
                </div>

                <div>
                  <div>{money(item.line_total)}</div>
                  <div className="muted">{money(item.unit_price)} / cái</div>
                </div>
              </li>
            ))}
          </ul>
        </section>
      ))}

      <div className="totals">
        <div className="totals__row">
          <span>Tạm tính</span>
          <span>{money(c?.subtotal)}</span>
        </div>
        <div className="totals__row totals__row--grand">
          <span>Tổng cộng</span>
          <span>{money(c?.total)}</span>
        </div>
        <p className="muted">
          Phí vận chuyển tính ở bước thanh toán, sau khi bạn nhập địa chỉ.
        </p>
      </div>

      <div className="actions">
        <Link href="/checkout">
          <Button>Tiến hành thanh toán</Button>
        </Link>
        <Link href="/">
          <Button variant="secondary">Mua thêm</Button>
        </Link>
      </div>
    </div>
  );
}
