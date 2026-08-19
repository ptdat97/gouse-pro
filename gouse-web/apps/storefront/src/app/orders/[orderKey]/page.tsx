"use client";

import {
  getMyOrder,
  isApiError,
  listOrderShipments,
  type OrderShipments,
  type OrderView,
} from "@fc/api-client";
import { Alert, Badge } from "@fc/ui";
import { useSearchParams } from "next/navigation";
import * as React from "react";

import {
  dateTime,
  money,
  orderStatusLabel,
  shipmentStatusLabel,
} from "@/lib/format";
import { useShop } from "@/lib/shop";

/**
 * Chi tiết đơn hàng.
 *
 * # TRANG ghép hai nguồn, không phải một endpoint
 *
 * `order` giữ dòng hàng và tiền; `fulfillment` giữ tiến độ giao. Hai module
 * không gọi được nhau (fulfillment đã phụ thuộc order), nên trang gọi hai
 * endpoint và khớp `order_line_ids` với `lines` đã có — không cần lượt gọi
 * thứ ba để lấy tên sản phẩm.
 *
 * Lô giao hỏng KHÔNG làm mất cả trang: khách vẫn thấy đơn của mình, chỉ
 * thiếu phần tiến độ.
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
  const [shipments, setShipments] = React.useState<OrderShipments["data"]>([]);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await getMyOrder(api, orderKey, phone);
        if (cancelled) return;
        setOrder(res);

        try {
          const s = await listOrderShipments(api, orderKey, phone);
          if (!cancelled) setShipments(s.data ?? []);
        } catch {
          // Tiến độ giao hỏng thì bỏ qua phần đó — đơn hàng vẫn hiển thị.
          if (!cancelled) setShipments([]);
        }
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

      <section className="panel">
        <h2>Tiến độ giao hàng</h2>

        {shipments.length === 0 ? (
          <p className="muted">
            Đơn chưa được tách thành gói giao. Thông tin sẽ hiện ở đây khi
            nhà bán bắt đầu chuẩn bị hàng.
          </p>
        ) : (
          shipments.map((s) => {
            // Khớp với dòng hàng ĐÃ CÓ từ getOrder — không gọi thêm gì.
            const inThisPackage = (order.lines ?? []).filter((l) =>
              (s.order_line_ids ?? []).includes(l.order_line_id),
            );

            return (
              <div key={s.fulfillment_number} className="group">
                <p className="group__seller">
                  Gói {s.fulfillment_number} ·{" "}
                  <Badge>{shipmentStatusLabel(s.status)}</Badge>
                </p>

                {s.tracking_number && (
                  <p className="muted">
                    Mã vận đơn {s.tracking_number}
                    {s.shipping_provider ? ` · ${s.shipping_provider}` : ""}
                  </p>
                )}
                {s.delivered_at && (
                  <p className="muted">Đã giao {dateTime(s.delivered_at)}</p>
                )}

                <ul className="lines">
                  {inThisPackage.map((l) => (
                    <li key={l.order_line_id} className="line">
                      <div>
                        {l.product_name}
                        {l.variant_description && (
                          <span className="muted"> · {l.variant_description}</span>
                        )}
                      </div>
                      <div className="muted">SL {l.quantity}</div>
                    </li>
                  ))}
                </ul>
              </div>
            );
          })
        )}
      </section>
    </div>
  );
}
