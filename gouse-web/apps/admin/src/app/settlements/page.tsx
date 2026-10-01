"use client";

import {
  confirmSettlement,
  isApiError,
  listSettlements,
  markSettlementPaid,
  type AdminSettlement,
} from "@fc/api-client";
import { Alert, Badge, Button, ReasonDialog, Table, type Column } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime, money, shortId } from "@/lib/format";
import { useSession } from "@/lib/session";
import { settlementTone } from "@/lib/status";

/**
 * Trang ĐỢT ĐỐI SOÁT — đường tiền RA nhà bán.
 *
 * # Vì sao trang này tồn tại
 *
 * `DoiSoat.XacNhan` và `DanhDauDaTra` có từ migration 000036 và tới
 * 01/10/2026 không lời gọi nào — mọi đợt nằm `DRAFT` vĩnh viễn, nhà bán
 * không bao giờ được chi (P3-89).
 *
 * Hai endpoint đã dựng cùng ngày. Không có trang này thì chúng vẫn là hai
 * endpoint không ai gọi — đúng dạng lỗi vừa sửa, chỉ dịch lên một tầng.
 *
 * # Hai bước, không một nút
 *
 *   DRAFT ──Chốt──► CONFIRMED ──Ghi nhận đã chuyển──► PAID
 *
 * Bước một là lúc con số thành CAM KẾT với nhà bán. Bước hai là lúc tiền rời
 * nền tảng. Giao diện giữ đúng hai bước ấy: một nút cho mỗi trạng thái, và
 * không có nút nào làm cả hai.
 *
 * # Lý do đi qua `ReasonDialog`, không phải một ô `confirm()`
 *
 * Backend đòi lý do ≥ 20 ký tự và từ chối giá trị rác. Dùng cùng component
 * với hủy đơn và điều chỉnh sổ cái thay vì viết ô nhập riêng: một ô riêng sẽ
 * lệch quy tắc, và người dùng gặp lỗi 400 mà không hiểu vì sao.
 */
export default function SettlementsPage() {
  return (
    <Shell>
      <SettlementsView />
    </Shell>
  );
}

/** Thao tác đang chờ xác nhận. */
interface PendingAction {
  row: AdminSettlement;
  buoc: "confirm" | "mark-paid";
}

function SettlementsView() {
  const { api } = useSession();
  const [status, setStatus] = React.useState("");
  const [rows, setRows] = React.useState<AdminSettlement[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const [pending, setPending] = React.useState<PendingAction | null>(null);
  const [actionError, setActionError] = React.useState<string | null>(null);
  const [submitting, setSubmitting] = React.useState(false);

  const load = React.useCallback(
    async (st: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await listSettlements(api, { status: st || undefined });
        setRows(res.data ?? []);
      } catch (e) {
        setError(errorText(e));
      } finally {
        setLoading(false);
      }
    },
    [api],
  );

  React.useEffect(() => {
    void load(status);
  }, [load, status]);

  async function onConfirm(reason: string) {
    if (!pending) return;
    setSubmitting(true);
    setActionError(null);
    try {
      // Khóa idempotency MỚI cho mỗi lần bấm, không dùng lại mã đợt: dùng
      // lại nghĩa là một lần bấm thứ hai có chủ ý (sau khi sửa lý do) sẽ
      // nhận lại phản hồi cũ.
      const key = crypto.randomUUID();
      const ky =
        pending.buoc === "confirm" ? confirmSettlement : markSettlementPaid;
      await ky(api, pending.row.id, reason, key);
      setPending(null);
      await load(status);
    } catch (e) {
      setActionError(errorText(e));
    } finally {
      setSubmitting(false);
    }
  }

  const columns: Column<AdminSettlement>[] = [
    {
      header: "Đợt",
      cell: (r) => <span className="mono">{shortId(r.id)}</span>,
    },
    {
      header: "Gian hàng",
      cell: (r) => <span className="mono">{shortId(r.seller_id)}</span>,
    },
    {
      header: "Trạng thái",
      cell: (r) => <Badge tone={settlementTone(r.status)}>{r.status}</Badge>,
    },
    {
      header: "Tổng gom",
      numeric: true,
      cell: (r) => money(r.gross_amount),
    },
    {
      // Hiện CẢ phần bị trừ, không chỉ số thực chi: nhà bán thấy số nhỏ hơn
      // tổng và sẽ hỏi vì sao. Người vận hành phải trả lời được ngay trên
      // màn hình này.
      header: "Trừ nợ",
      numeric: true,
      cell: (r) => money(r.deficit_amount),
    },
    {
      header: "Thực chi",
      numeric: true,
      cell: (r) => <strong>{money(r.net_amount)}</strong>,
    },
    {
      header: "Người ký",
      cell: (r) => <NguoiKy row={r} />,
    },
    {
      header: "",
      cell: (r) => (
        <NutKy
          row={r}
          onKy={(buoc) => {
            setActionError(null);
            setPending({ row: r, buoc });
          }}
        />
      ),
    },
  ];

  return (
    <>
      <div className="page__header">
        <div>
          <h1>Đợt đối soát</h1>
          <p className="page__lead">
            Đường tiền ra nhà bán. Hai bước, mỗi bước cần lý do và được ghi
            vào nhật ký thao tác.
          </p>
        </div>
      </div>

      <div className="toolbar">
        {[
          { v: "", label: "Tất cả" },
          { v: "DRAFT", label: "Chờ chốt" },
          { v: "CONFIRMED", label: "Chờ chuyển tiền" },
          { v: "PAID", label: "Đã trả" },
        ].map((t) => (
          <Button
            key={t.v}
            type="button"
            variant={status === t.v ? "primary" : undefined}
            onClick={() => setStatus(t.v)}
          >
            {t.label}
          </Button>
        ))}
      </div>

      {error && <Alert tone="danger">{error}</Alert>}

      {loading ? (
        <p>Đang tải…</p>
      ) : (
        <Table
          columns={columns}
          rows={rows}
          rowKey={(r) => r.id}
          empty="Không có đợt đối soát nào."
        />
      )}

      {pending && (
        <ReasonDialog
          open
          onOpenChange={(open) => {
            if (!open) setPending(null);
          }}
          title={
            pending.buoc === "confirm"
              ? "Chốt đợt đối soát"
              : "Ghi nhận đã chuyển tiền"
          }
          warning={
            pending.buoc === "confirm"
              ? "Chốt xong là một CAM KẾT với nhà bán — số này sẽ hiện trên màn hình của họ."
              : "Bước này KHÔNG chuyển tiền. Chỉ bấm sau khi đã chuyển khoản xong."
          }
          impact={
            <>
              Gian hàng <span className="mono">{shortId(pending.row.seller_id)}</span>{" "}
              · thực chi <strong>{money(pending.row.net_amount)}</strong>
            </>
          }
          confirmLabel={
            pending.buoc === "confirm" ? "Chốt đợt" : "Đã chuyển tiền"
          }
          confirmTone="danger"
          serverError={actionError}
          submitting={submitting}
          onConfirm={(reason) => void onConfirm(reason)}
        />
      )}
    </>
  );
}

/**
 * Nút ký của MỘT trạng thái.
 *
 * `PAID` không có nút: vòng đời đã hết, và một nút không làm gì là một nút
 * người dùng sẽ bấm rồi không hiểu vì sao im lặng.
 */
function NutKy({
  row,
  onKy,
}: {
  row: AdminSettlement;
  onKy: (buoc: "confirm" | "mark-paid") => void;
}) {
  if (row.status === "DRAFT") {
    // Đợt không có số dương KHÔNG chốt được — backend từ chối, nên nút phải
    // mờ chứ không để người vận hành bấm rồi nhận lỗi.
    const chotDuoc = (row.net_amount?.amount ?? 0) > 0;
    return (
      <Button
        type="button"
        variant="primary"
        disabled={!chotDuoc}
        title={chotDuoc ? undefined : "Không có số dương để chi trả"}
        onClick={() => onKy("confirm")}
      >
        Chốt
      </Button>
    );
  }
  if (row.status === "CONFIRMED") {
    return (
      <Button type="button" onClick={() => onKy("mark-paid")}>
        Đã chuyển tiền
      </Button>
    );
  }
  return null;
}

/** Người ký của hai bước, mốc thời gian kèm theo. */
function NguoiKy({ row }: { row: AdminSettlement }) {
  if (!row.confirmed_by && !row.paid_by) return <>—</>;
  return (
    <div className="muted">
      {row.confirmed_by && (
        <small>
          Chốt: <span className="mono">{shortId(row.confirmed_by)}</span>
          {" · "}
          {dateTime(row.confirmed_at)}
        </small>
      )}
      {row.paid_by && (
        <small>
          Trả: <span className="mono">{shortId(row.paid_by)}</span>
          {" · "}
          {dateTime(row.paid_at)}
        </small>
      )}
    </div>
  );
}

function errorText(e: unknown): string {
  if (isApiError(e)) return e.message;
  return "Không kết nối được máy chủ. Thử lại sau.";
}
