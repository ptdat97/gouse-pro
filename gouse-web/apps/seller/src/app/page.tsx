"use client";

import {
  isApiError,
  listMyFulfillmentOrders,
  shipFulfillmentOrder,
  type MyFulfillmentOrders,
} from "@fc/api-client";
import { Alert, Badge, Button, Field, Input } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime, money } from "@/lib/format";
import { foStatusLabel, foTone } from "@/lib/status";
import { useSession } from "@/lib/session";

type FO = NonNullable<MyFulfillmentOrders["data"]>[number];

/**
 * Việc cần làm — màn hình chính của nhà bán.
 *
 * # Mỗi đơn là một PHIẾU GIAO HÀNG hoàn chỉnh
 *
 * Nhặt gì (tên sản phẩm, size), gửi đi đâu (địa chỉ, số điện thoại), và một
 * nút bàn giao. Nhà bán KHÔNG được xem đơn hàng gốc — ở đó có hàng của nhà
 * bán khác, email khách và tổng tiền cả đơn.
 *
 * # Một nút, không phải bốn bước
 *
 * Backend đi qua mọi bước trung gian (xác nhận → đóng gói → bàn giao). Cửa
 * hàng nhỏ đóng gói ở bàn làm việc rồi ghi nhận đã gửi — họ không có quy
 * trình kho nhiều bước.
 */
export default function WorkQueuePage() {
  return (
    <Shell>
      <WorkQueue />
    </Shell>
  );
}

function WorkQueue() {
  const { api } = useSession();
  const [rows, setRows] = React.useState<FO[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listMyFulfillmentOrders(api);
      setRows(res.data ?? []);
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không tải được danh sách");
    } finally {
      setLoading(false);
    }
  }, [api]);

  React.useEffect(() => {
    void load();
  }, [load]);

  if (loading) return <p>Đang tải…</p>;

  // Đơn CHƯA bàn giao lên trước: đó là việc cần làm hôm nay. Đơn đã gửi
  // vẫn hiện để nhà bán tra mã vận đơn khi khách hỏi.
  const pending = rows.filter((f) => !f.shipped_at);
  const shipped = rows.filter((f) => f.shipped_at);

  return (
    <div>
      <h1>Việc cần làm</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      {pending.length === 0 ? (
        <p className="muted">Không có đơn nào chờ xử lý.</p>
      ) : (
        pending.map((f) => <PackingSlip key={f.id} fo={f} onShipped={load} />)
      )}

      {shipped.length > 0 && (
        <>
          <h2>Đã bàn giao</h2>
          {shipped.map((f) => (
            <section key={f.id} className="panel">
              <p>
                <strong>{f.fulfillment_number}</strong>{" "}
                <Badge tone={foTone(f.status)}>{foStatusLabel(f.status)}</Badge>
              </p>
              <p className="muted">
                {f.shipping_provider} · {f.tracking_number} ·{" "}
                {dateTime(f.shipped_at)}
              </p>
            </section>
          ))}
        </>
      )}
    </div>
  );
}

/**
 * Một phiếu giao hàng.
 *
 * Địa chỉ có thể VẮNG MẶT với đơn tách trước khi hệ thống lưu nó. Nói rõ
 * điều đó thay vì in phiếu trống — nhà bán sẽ gửi hàng đi đâu đó sai.
 */
function PackingSlip({ fo, onShipped }: { fo: FO; onShipped: () => void }) {
  const { api } = useSession();
  const [tracking, setTracking] = React.useState("");
  const [provider, setProvider] = React.useState("GHN");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  async function onShip(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await shipFulfillmentOrder(api, fo.id, tracking.trim(), provider.trim());
      onShipped();
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không ghi nhận được bàn giao");
      setBusy(false);
    }
  }

  const addr = fo.shipping_address;

  return (
    <section className="panel">
      <p>
        <strong>{fo.fulfillment_number}</strong>{" "}
        <Badge tone={foTone(fo.status)}>{foStatusLabel(fo.status)}</Badge>{" "}
        <span className="muted">nhận lúc {dateTime(fo.created_at)}</span>
      </p>

      <h3>Nhặt hàng</h3>
      <ul className="lines">
        {(fo.items ?? []).map((i, idx) => (
          <li key={idx} className="line">
            <div>
              <strong>{i.product_name}</strong>
              {i.variant_description && (
                <span className="muted"> · {i.variant_description}</span>
              )}
            </div>
            <div>× {i.quantity}</div>
          </li>
        ))}
      </ul>

      <h3>Gửi tới</h3>
      {addr ? (
        <p>
          {addr.recipient_name} · {addr.phone}
          <br />
          {[addr.street_address, addr.ward, addr.district, addr.province]
            .filter(Boolean)
            .join(", ")}
        </p>
      ) : (
        <Alert tone="warning">
          Đơn này chưa có địa chỉ giao. Liên hệ nền tảng trước khi gửi hàng —
          đừng đoán.
        </Alert>
      )}

      <p className="muted">
        Bạn nhận được {money(fo.seller_payable)} (đã trừ hoa hồng{" "}
        {money(fo.commission_amount)}).
      </p>

      {error && <Alert tone="danger">{error}</Alert>}

      <form onSubmit={onShip}>
        <div className="form-row">
          <Field label="Đơn vị vận chuyển" htmlFor={`prov-${fo.id}`}>
            <Input
              id={`prov-${fo.id}`}
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
              required
            />
          </Field>
          <Field
            label="Mã vận đơn"
            htmlFor={`track-${fo.id}`}
            hint="Bắt buộc — không có mã thì không ai tra được hàng đang ở đâu"
          >
            <Input
              id={`track-${fo.id}`}
              value={tracking}
              onChange={(e) => setTracking(e.target.value)}
              required
            />
          </Field>
        </div>

        <div className="actions">
          <Button type="submit" variant="primary" disabled={busy || !addr}>
            {busy ? "Đang ghi nhận…" : "Đã bàn giao vận chuyển"}
          </Button>
        </div>
      </form>
    </section>
  );
}
