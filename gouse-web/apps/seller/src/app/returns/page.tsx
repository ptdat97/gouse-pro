"use client";

import {
  approveReturn,
  inspectReturn,
  isApiError,
  listMyReturns,
  receiveReturn,
  rejectReturn,
  type InspectLine,
  type MyReturns,
} from "@fc/api-client";
import { Alert, Badge, Button, Field, Input } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime, money } from "@/lib/format";
import { returnReasonLabel, returnStatusLabel, returnTone } from "@/lib/status";
import { useSession } from "@/lib/session";

type Return = NonNullable<MyReturns["data"]>[number];

/**
 * Trả hàng — bốn bước, và mỗi bước có hậu quả khác nhau.
 *
 * # Vì sao màn hình này tồn tại
 *
 * Backend có đủ cả bốn bước từ lâu (duyệt · từ chối · nhận hàng · kiểm
 * định) và không ai bấm được: sáu tuyến của luồng này sống ngoài đặc tả
 * OpenAPI cho tới 17/09, nên `openapi-typescript` không sinh kiểu và trang
 * này không gọi được theo cách có kiểu. Xem P3-56.
 *
 * # Thứ tự các bước KHÔNG tùy ý
 *
 *	Chờ duyệt   → duyệt hoặc từ chối
 *	Đã duyệt    → nhận hàng  (đây là bước ĐI TIỀN)
 *	Đã nhận     → kiểm định  (đây là bước quyết định hàng bán lại được không)
 *
 * Giao diện chỉ hiện nút của bước HIỆN TẠI. Hiện cả bốn rồi để backend từ
 * chối là bắt nhà bán học quy trình bằng cách bấm sai.
 */
export default function ReturnsPage() {
  return (
    <Shell>
      <Returns />
    </Shell>
  );
}

function Returns() {
  const { api } = useSession();
  const [rows, setRows] = React.useState<Return[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listMyReturns(api);
      setRows(res.data ?? []);
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không tải được danh sách trả hàng");
    } finally {
      setLoading(false);
    }
  }, [api]);

  React.useEffect(() => {
    void load();
  }, [load]);

  if (loading) return <p>Đang tải…</p>;

  // Việc CẦN LÀM lên trước, theo đúng thứ tự khách đang chờ.
  //
  // Yêu cầu đã xong vẫn hiện: nhà bán cần tra lại khi khách hỏi "tiền của
  // tôi đâu", và một danh sách chỉ có việc chưa làm không trả lời được.
  const canLam = rows.filter((r) =>
    ["REQUESTED", "APPROVED", "RECEIVED"].includes(r.status ?? ""),
  );
  const xong = rows.filter(
    (r) => !["REQUESTED", "APPROVED", "RECEIVED"].includes(r.status ?? ""),
  );

  return (
    <div>
      <h1>Trả hàng</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      {rows.length === 0 && <p className="muted">Chưa có yêu cầu trả hàng nào.</p>}

      {canLam.map((r) => (
        <ReturnCard key={r.id} r={r} onDone={load} />
      ))}

      {xong.length > 0 && (
        <>
          <h2>Đã xử lý</h2>
          {xong.map((r) => (
            <ReturnCard key={r.id} r={r} onDone={load} />
          ))}
        </>
      )}
    </div>
  );
}

function ReturnCard({ r, onDone }: { r: Return; onDone: () => void }) {
  const { api } = useSession();
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [rejecting, setRejecting] = React.useState(false);
  const [reason, setReason] = React.useState("");

  async function chay(lam: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await lam();
      onDone();
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không thực hiện được");
      setBusy(false);
    }
  }

  const items = r.items ?? [];

  return (
    <section className="panel">
      <p>
        <strong>{r.id}</strong>{" "}
        <Badge tone={returnTone(r.status)}>{returnStatusLabel(r.status)}</Badge>{" "}
        <span className="muted">gửi lúc {dateTime(r.requested_at)}</span>
      </p>

      <p>
        Lý do: <strong>{returnReasonLabel(r.reason_code)}</strong>
        {r.customer_note && <span className="muted"> · “{r.customer_note}”</span>}
      </p>

      <ul className="lines">
        {items.map((i, idx) => (
          <li key={idx} className="line">
            <div>
              <span className="muted">{i.sku_id}</span> × {i.quantity}
              {i.reason_detail && (
                <span className="muted"> · {i.reason_detail}</span>
              )}
            </div>
            <div>{money(i.refund)}</div>
          </li>
        ))}
      </ul>

      <p>
        Hoàn cho khách: <strong>{money(r.refund_amount)}</strong>
      </p>

      {r.reject_reason && (
        <p className="muted">Lý do từ chối: {r.reject_reason}</p>
      )}

      {error && <Alert tone="danger">{error}</Alert>}

      {/* Chờ duyệt: hai lựa chọn, và chúng KHÔNG cân nhau. */}
      {r.status === "REQUESTED" && !rejecting && (
        <p>
          <Button disabled={busy} onClick={() => void chay(() => approveReturn(api, r.id!))}>
            Duyệt
          </Button>{" "}
          <Button variant="secondary" disabled={busy} onClick={() => setRejecting(true)}>
            Từ chối
          </Button>
        </p>
      )}

      {/*
        Từ chối BẮT BUỘC có lý do, và ô nhập hiện ra TRƯỚC khi bấm được.
        Một nút "Từ chối" bấm phát ăn ngay sẽ sinh ra những lần từ chối
        không ai giải thích được cho khách.
      */}
      {r.status === "REQUESTED" && rejecting && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void chay(() => rejectReturn(api, r.id!, reason.trim()));
          }}
        >
          <Field label="Lý do từ chối (khách sẽ đọc được)" htmlFor={`tuchoi-${r.id}`}>
            <Input
              id={`tuchoi-${r.id}`}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              required
              minLength={1}
              placeholder="Hàng đã qua sử dụng, không còn tem mác"
            />
          </Field>
          <p>
            <Button type="submit" disabled={busy || reason.trim() === ""}>
              Gửi từ chối
            </Button>{" "}
            <Button
              type="button"
              variant="secondary"
              disabled={busy}
              onClick={() => setRejecting(false)}
            >
              Quay lại
            </Button>
          </p>
        </form>
      )}

      {/* Đã duyệt: chỉ bấm khi hàng ĐÃ về tới nơi — đây là bước đi tiền. */}
      {r.status === "APPROVED" && (
        <p>
          <Button disabled={busy} onClick={() => void chay(() => receiveReturn(api, r.id!))}>
            Đã nhận được hàng — hoàn tiền
          </Button>
          <span className="muted">
            {" "}
            Bấm sau khi hàng về tới kho. Tiền đi ngay ở bước này.
          </span>
        </p>
      )}

      {(r.status === "RECEIVED" || r.status === "REFUNDED") && (
        <Inspect r={r} busy={busy} onRun={chay} />
      )}
    </section>
  );
}

/**
 * Kiểm định theo TỪNG DÒNG.
 *
 * Một yêu cầu trả hai món hoàn toàn có thể một món bán lại được và một
 * món hỏng, nên không có nút "duyệt tất".
 *
 * Mặc định là CHƯA chọn, không phải "đạt". Mặc định đạt sẽ đưa hàng hỏng
 * trở lại kệ chỉ vì người kiểm bấm nhanh — và bán lại hàng hỏng cho khách
 * khác gây thiệt hại uy tín lớn hơn nhiều lần giá trị món hàng.
 */
function Inspect({
  r,
  busy,
  onRun,
}: {
  r: Return;
  busy: boolean;
  onRun: (lam: () => Promise<unknown>) => void;
}) {
  const { api } = useSession();
  const items = r.items ?? [];
  const daKiem = items.every((i) => i.inspection);

  const [ketQua, setKetQua] = React.useState<Record<string, boolean>>({});
  const [ghiChu, setGhiChu] = React.useState<Record<string, string>>({});

  if (daKiem) {
    return (
      <p className="muted">
        Đã kiểm định:{" "}
        {items
          .map((i) => `${i.sku_id}: ${i.inspection === "PASSED" ? "đạt" : "hỏng"}`)
          .join(" · ")}
      </p>
    );
  }

  const duChon = items.every((i) => ketQua[i.order_line_id!] !== undefined);

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const lines: InspectLine[] = items.map((i) => ({
          order_line_id: i.order_line_id!,
          passed: ketQua[i.order_line_id!] === true,
          note: ghiChu[i.order_line_id!]?.trim() || undefined,
        }));
        onRun(() => inspectReturn(api, r.id!, lines));
      }}
    >
      <h3>Kiểm định hàng về</h3>
      {items.map((i) => {
        const id = i.order_line_id!;
        return (
          <div key={id} className="line">
            <div>
              <span className="muted">{i.sku_id}</span> × {i.quantity}
            </div>
            <div>
              <label>
                <input
                  type="radio"
                  name={`kq-${id}`}
                  checked={ketQua[id] === true}
                  onChange={() => setKetQua({ ...ketQua, [id]: true })}
                />{" "}
                Bán lại được
              </label>{" "}
              <label>
                <input
                  type="radio"
                  name={`kq-${id}`}
                  checked={ketQua[id] === false}
                  onChange={() => setKetQua({ ...ketQua, [id]: false })}
                />{" "}
                Hỏng
              </label>
            </div>
            {ketQua[id] === false && (
              <Field label="Hỏng thế nào (bắt buộc)" htmlFor={`ghichu-${id}`}>
                <Input
                  id={`ghichu-${id}`}
                  value={ghiChu[id] ?? ""}
                  onChange={(e) => setGhiChu({ ...ghiChu, [id]: e.target.value })}
                  required
                  placeholder="Rách vai trái, mất cúc"
                />
              </Field>
            )}
          </div>
        );
      })}
      <p>
        <Button type="submit" disabled={busy || !duChon}>
          Ghi kết quả kiểm định
        </Button>
        {!duChon && (
          <span className="muted"> Chọn kết quả cho từng món trước.</span>
        )}
      </p>
    </form>
  );
}
