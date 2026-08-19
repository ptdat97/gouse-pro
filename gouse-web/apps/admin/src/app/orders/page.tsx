"use client";

import {
  cancelOrder,
  getOrder,
  isApiError,
  listOrders,
  type OrderDetail,
} from "@fc/api-client";
import {
  Alert,
  Badge,
  Button,
  Field,
  Input,
  ReasonDialog,
  Table,
  type Column,
} from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime, money } from "@/lib/format";
import { orderTone } from "@/lib/status";
import { useSession } from "@/lib/session";

/**
 * Trang hỗ trợ khách hàng.
 *
 * # Cổng audit trước khi xem dữ liệu khách
 *
 * Danh sách KHÔNG chứa tên hay số điện thoại. Muốn xem chi tiết phải nhập
 * LÝ DO, và mỗi lần xem ghi một bản ghi vào nhật ký thao tác — theo
 * admin-api.md mục 6.
 *
 * Đây là lý do trang này có hai bước thay vì một: nếu bấm phát là thấy hết,
 * thì việc ghi vết mất ý nghĩa vì ai cũng mở mọi đơn cho tiện.
 */

interface Row {
  id: string;
  order_number: string;
  status: string;
  total: { amount: number; currency: string };
  line_count: number;
  placed_at: string;
}

export default function OrdersPage() {
  return (
    <Shell>
      <OrdersView />
    </Shell>
  );
}

function OrdersView() {
  const { api } = useSession();
  const [orderNumber, setOrderNumber] = React.useState("");
  const [rows, setRows] = React.useState<Row[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [openId, setOpenId] = React.useState<string | null>(null);

  const load = React.useCallback(
    async (num?: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await listOrders(api, {
          order_number: num || undefined,
          limit: 50,
        });
        setRows((res.data ?? []) as Row[]);
      } catch (e) {
        setError(errorText(e));
      } finally {
        setLoading(false);
      }
    },
    [api],
  );

  React.useEffect(() => {
    void load();
  }, [load]);

  const columns: Column<Row>[] = [
    {
      header: "Mã đơn",
      cell: (r) => <span className="mono">{r.order_number}</span>,
    },
    {
      header: "Trạng thái",
      cell: (r) => <Badge tone={orderTone(r.status)}>{r.status}</Badge>,
    },
    { header: "Số dòng", numeric: true, cell: (r) => r.line_count },
    { header: "Tổng tiền", numeric: true, cell: (r) => money(r.total) },
    { header: "Đặt lúc", cell: (r) => dateTime(r.placed_at) },
  ];

  return (
    <>
      <div className="page__header">
        <div>
          <h1>Đơn hàng</h1>
          <p className="page__lead">
            Tra cứu và hỗ trợ khách. Xem chi tiết cần nêu lý do.
          </p>
        </div>
      </div>

      <form
        className="toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void load(orderNumber);
        }}
      >
        <Field
          label="Mã đơn"
          htmlFor="order_number"
          hint="Mã khách đọc qua điện thoại, ví dụ FC-2026-08-000123"
        >
          <Input
            value={orderNumber}
            onChange={(e) => setOrderNumber(e.target.value)}
            placeholder="FC-…"
          />
        </Field>
        <Button type="submit" variant="primary">
          Tìm
        </Button>
        <Button
          type="button"
          onClick={() => {
            setOrderNumber("");
            void load();
          }}
        >
          Xóa lọc
        </Button>
      </form>

      {error && <Alert tone="danger">{error}</Alert>}

      {loading ? (
        <p>Đang tải…</p>
      ) : (
        <Table
          columns={columns}
          rows={rows}
          rowKey={(r) => r.id}
          onRowClick={(r) => setOpenId(r.id)}
          empty="Không tìm thấy đơn nào."
        />
      )}

      {openId && (
        <OrderPanel
          orderId={openId}
          onClose={() => setOpenId(null)}
          onChanged={() => {
            setOpenId(null);
            void load(orderNumber);
          }}
        />
      )}
    </>
  );
}

/**
 * Chi tiết đơn — mở bằng cổng lý do.
 *
 * Dùng ReasonDialog cho cả BƯỚC XEM, không chỉ cho thao tác ghi: cùng một
 * yêu cầu (lý do ≥ 20 ký tự) nên dùng cùng một component, thay vì viết một
 * ô nhập riêng dễ lệch quy tắc.
 */
function OrderPanel({
  orderId,
  onClose,
  onChanged,
}: {
  orderId: string;
  onClose: () => void;
  onChanged: () => void;
}) {
  const { api } = useSession();
  const [order, setOrder] = React.useState<OrderDetail | null>(null);
  const [gateOpen, setGateOpen] = React.useState(true);
  const [gateError, setGateError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(false);

  const [cancelOpen, setCancelOpen] = React.useState(false);
  const [cancelError, setCancelError] = React.useState<string | null>(null);
  const [submitting, setSubmitting] = React.useState(false);

  async function onOpenWithReason(reason: string) {
    setLoading(true);
    setGateError(null);
    try {
      setOrder(await getOrder(api, orderId, reason));
      setGateOpen(false);
    } catch (e) {
      setGateError(errorText(e));
    } finally {
      setLoading(false);
    }
  }

  async function onCancel(reason: string) {
    setSubmitting(true);
    setCancelError(null);
    try {
      await cancelOrder(api, orderId, reason);
      setCancelOpen(false);
      onChanged();
    } catch (e) {
      setCancelError(errorText(e));
    } finally {
      setSubmitting(false);
    }
  }

  if (!order) {
    return (
      <ReasonDialog
        open={gateOpen}
        onOpenChange={(v) => {
          setGateOpen(v);
          if (!v) onClose();
        }}
        title="Xem chi tiết đơn hàng"
        impact={
          <span>
            Chi tiết đơn chứa <strong>tên, số điện thoại và địa chỉ</strong>{" "}
            của khách. Lần truy cập này sẽ được ghi vào nhật ký thao tác kèm
            tên bạn.
          </span>
        }
        confirmLabel="Xem chi tiết"
        confirmTone="primary"
        submitting={loading}
        serverError={gateError}
        onConfirm={(reason) => void onOpenWithReason(reason)}
      />
    );
  }

  const cancellable =
    order.status !== "CANCELLED" &&
    order.status !== "COMPLETED" &&
    order.status !== "DELIVERED";

  return (
    <div className="detail" style={{ marginTop: "var(--space-6)" }}>
      <div className="page__header">
        <h2 className="mono">{order.order_number}</h2>
        <Button onClick={onClose}>Đóng</Button>
      </div>

      <div className="detail__grid">
        <Item label="Trạng thái">
          <Badge tone={orderTone(order.status)}>{order.status}</Badge>
        </Item>
        <Item label="Tổng tiền">{money(order.total)}</Item>
        <Item label="Đặt lúc">{dateTime(order.placed_at)}</Item>
        <Item label="Người nhận">{order.shipping?.recipient_name || "—"}</Item>
        <Item label="Điện thoại">{order.shipping?.phone || "—"}</Item>
        <Item label="Địa chỉ">
          {[
            order.shipping?.street,
            order.shipping?.ward,
            order.shipping?.district,
            order.shipping?.province,
          ]
            .filter(Boolean)
            .join(", ") || "—"}
        </Item>
      </div>

      <h3 style={{ marginTop: "var(--space-6)" }}>Dòng hàng</h3>
      <Table
        columns={[
          { header: "SKU", cell: (l) => <span className="mono">{l.sku_id ?? "—"}</span> },
          { header: "Nhà bán", cell: (l) => <span className="mono">{l.seller_id ?? "—"}</span> },
          { header: "SL", numeric: true, cell: (l) => l.quantity },
          { header: "Trạng thái", cell: (l) => l.status },
          { header: "Thành tiền", numeric: true, cell: (l) => money(l.line_total) },
        ]}
        rows={order.lines ?? []}
        rowKey={(l) => l.id ?? ""}
        empty="Đơn không có dòng hàng."
      />

      {/*
        Lô giao hàng và bút toán KHÔNG nằm trong response này — module order
        không được gọi fulfillment hay payment (phụ thuộc vòng). Nói rõ thay
        vì để nhân viên tưởng đơn không có lô giao nào.
      */}
      <Alert tone="info" title="Lô giao hàng và bút toán">
        Chưa hiển thị ở đây. Hai nhóm dữ liệu này thuộc module khác và cần
        endpoint riêng — xem backlog P1.5 (Seller Center) và P1.6.
      </Alert>

      {cancellable && (
        <div className="actions">
          <Button variant="danger" onClick={() => setCancelOpen(true)}>
            Hủy đơn
          </Button>
        </div>
      )}

      <ReasonDialog
        open={cancelOpen}
        onOpenChange={setCancelOpen}
        title={`Hủy đơn ${order.order_number}`}
        warning="Đơn đã hủy không khôi phục được."
        impact={
          <span>
            Hoàn tiền và nhả tồn kho chạy <strong>bất đồng bộ</strong> qua
            event — kiểm tra lại sổ cái và tồn kho sau vài giây.
          </span>
        }
        confirmLabel="Xác nhận hủy đơn"
        submitting={submitting}
        serverError={cancelError}
        onConfirm={(reason) => void onCancel(reason)}
      />
    </div>
  );
}

function Item({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <span className="detail__label">{label}</span>
      <div className="detail__value">{children}</div>
    </div>
  );
}

function errorText(e: unknown): string {
  if (isApiError(e)) {
    if (e.isForbidden) return "Bạn không có quyền thực hiện thao tác này.";
    return e.message;
  }
  return "Không kết nối được máy chủ.";
}
