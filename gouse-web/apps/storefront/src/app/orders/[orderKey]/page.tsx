"use client";

import { getMyOrder, isApiError, type OrderView } from "@fc/api-client";
import { Alert, Badge } from "@fc/ui";
import { useSearchParams } from "next/navigation";
import * as React from "react";

import { dateTime, money, orderStatusLabel } from "@/lib/format";
import { useShop } from "@/lib/shop";

/**
 * Chi tiết đơn hàng.
 *
 * # Response KHÔNG có tiến độ giao hàng
 *
 * Dữ liệu lô giao thuộc module `fulfillment`, và endpoint cho khách xem nó
 * CHƯA tồn tại (backlog P1.8). Trang này hiển thị thứ có thật — trạng thái
 * tổng hợp, dòng hàng, tiền, địa chỉ — thay vì để chỗ trống hứa hẹn.
 *
 * # 404 cho cả "không có" lẫn "không phải của bạn"
 *
 * Mã đơn tăng dần, nên hai thông báo khác nhau sẽ để lộ nền tảng bán bao
 * nhiêu đơn mỗi tháng. Thông báo ở đây phải nói được cả hai khả năng.
 */
export default function OrderDetailPage({
  params,
}: {
  params: Promise<{ orderKey: string }>;
}) {
  const { orderKey } = React.use(params);
  const { api } = useShop();
  const search = useSearchParams();
  const phone = search.get("phone") ?? undefined;
  const justPlaced = search.get("placed") === "1";

  const [order, setOrder] = React.useState<OrderView | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await getMyOrder(api, orderKey, phone);
        if (!cancelled) setOrder(res);
      } catch (e) {
        if (cancelled) return;
        setError(
          isApiError(e) && e.status === 404
            ? "Không tìm thấy đơn hàng. Kiểm tra lại mã đơn và số điện thoại."
            : isApiError(e)
              ? e.message
              : "Không tải được đơn hàng",
        );
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api, orderKey, phone]);

  if (error) return <Alert tone="danger">{error}</Alert>;
  if (!order) return <p className="muted">Đang tải…</p>;

  return (
    <div>
      {justPlaced && (
        <Alert tone="success">
          Đặt hàng thành công. Mã đơn của bạn là <strong>{order.order_number}</strong> —
          hãy lưu lại để tra cứu.
        </Alert>
      )}

      <h1>Đơn {order.order_number}</h1>
      <p>
        <Badge>{orderStatusLabel(order.status)}</Badge>{" "}
        <span className="muted">Đặt lúc {dateTime(order.placed_at)}</span>
      </p>

      <section className="panel">
        <h2>Sản phẩm</h2>
        <ul className="lines">
          {(order.lines ?? []).map((l) => (
            <li key={l.order_line_id} className="line">
              <div>
                <strong>{l.product_name}</strong>
                {l.variant_description && (
                  <div className="muted">{l.variant_description}</div>
                )}
                <div className="muted">
                  SL {l.quantity}
                  {l.status !== "ACTIVE" && " · đã hủy"}
                </div>
              </div>
              <div>{money(l.line_total)}</div>
            </li>
          ))}
        </ul>

        <div className="totals">
          <div className="totals__row">
            <span>Tạm tính</span>
            <span>{money(order.subtotal)}</span>
          </div>
          <div className="totals__row">
            <span>Phí vận chuyển</span>
            <span>{money(order.shipping_fee)}</span>
          </div>
          <div className="totals__row totals__row--grand">
            <span>Tổng cộng</span>
            <span>{money(order.total)}</span>
          </div>
        </div>
      </section>

      {order.shipping_address && (
        <section className="panel">
          <h2>Giao tới</h2>
          <p>
            {order.shipping_address.recipient_name}
            <br />
            {order.shipping_address.phone}
            <br />
            {[
              order.shipping_address.street_address,
              order.shipping_address.ward,
              order.shipping_address.district,
              order.shipping_address.province,
            ]
              .filter(Boolean)
              .join(", ")}
          </p>
        </section>
      )}

      <p className="muted">
        Tiến độ giao hàng theo từng gói sẽ hiển thị ở đây khi có.
      </p>
    </div>
  );
}
