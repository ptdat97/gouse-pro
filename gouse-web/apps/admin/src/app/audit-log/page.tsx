"use client";

import {
  isApiError,
  listAuditLog,
  type AuditResourceType,
} from "@fc/api-client";
import {
  Alert,
  Badge,
  Button,
  Field,
  Input,
  Select,
  Table,
  type Column,
} from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime } from "@/lib/format";
import { useSession } from "@/lib/session";

/**
 * Nhật ký thao tác — CHỈ ĐỌC.
 *
 * Trang này KHÔNG có nút sửa hay xóa, và sẽ không bao giờ có. Nếu sửa được,
 * nhật ký mất hết giá trị: một bản ghi kiểm toán chỉ đáng tin khi không ai —
 * kể cả người có quyền cao nhất — thay đổi được nó sau khi sự việc xảy ra.
 *
 * Backend cũng không cung cấp đường nào để sửa: `platform/audit` chỉ có
 * Write và Query, và database có trigger chặn UPDATE/DELETE.
 */

interface Row {
  id: string;
  actor_type: string;
  actor_id?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  reason?: string;
  request_id?: string;
  occurred_at: string;
}

const RESOURCE_TYPES: AuditResourceType[] = [
  "LEDGER",
  "INVENTORY",
  "SELLER",
  "CREATOR",
  "CUSTOMER",
  "CONTENT",
  "ORDER",
  "CONFIG",
];

export default function AuditLogPage() {
  return (
    <Shell>
      <AuditLogView />
    </Shell>
  );
}

function AuditLogView() {
  const { api } = useSession();
  const [resourceType, setResourceType] = React.useState<
    AuditResourceType | ""
  >("");
  const [action, setAction] = React.useState("");
  const [from, setFrom] = React.useState("");
  const [to, setTo] = React.useState("");

  const [rows, setRows] = React.useState<Row[]>([]);
  const [cursor, setCursor] = React.useState<string | undefined>();
  const [hasMore, setHasMore] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const load = React.useCallback(
    async (opts: { append?: boolean; cursor?: string } = {}) => {
      setLoading(true);
      setError(null);
      try {
        const res = await listAuditLog(api, {
          resource_type: resourceType === "" ? undefined : resourceType,
          action: action || undefined,
          from: from || undefined,
          to: to || undefined,
          cursor: opts.cursor,
          limit: 25,
        });
        const data = (res.data ?? []) as Row[];
        setRows((prev) => (opts.append ? [...prev, ...data] : data));
        setCursor(res.pagination?.next_cursor ?? undefined);
        setHasMore(Boolean(res.pagination?.next_cursor));
      } catch (e) {
        setError(errorText(e));
      } finally {
        setLoading(false);
      }
    },
    [api, resourceType, action, from, to],
  );

  React.useEffect(() => {
    void load();
  }, [load]);

  const columns: Column<Row>[] = [
    { header: "Thời điểm", cell: (r) => dateTime(r.occurred_at) },
    {
      header: "Người thực hiện",
      cell: (r) =>
        r.actor_type === "SYSTEM" ? (
          <Badge>Hệ thống</Badge>
        ) : (
          <span className="mono">{r.actor_id || "—"}</span>
        ),
    },
    {
      header: "Hành động",
      cell: (r) => <span className="mono">{r.action}</span>,
    },
    {
      header: "Tài nguyên",
      cell: (r) => (
        <>
          <div>{r.resource_type}</div>
          {r.resource_id && (
            <div className="mono" style={{ color: "var(--color-text-muted)" }}>
              {r.resource_id}
            </div>
          )}
        </>
      ),
    },
    {
      header: "Lý do",
      // Lý do là NỘI DUNG CHÍNH của nhật ký — không cắt ngắn, không giấu
      // sau tooltip. Đọc lại sau sáu tháng thì đây là thứ duy nhất giải
      // thích được vì sao thao tác đó xảy ra.
      cell: (r) => r.reason || <span style={{ color: "var(--color-text-muted)" }}>—</span>,
    },
  ];

  return (
    <>
      <div className="page__header">
        <div>
          <h1>Nhật ký thao tác</h1>
          <p className="page__lead">
            Chỉ đọc. Không có cách nào sửa hoặc xóa bản ghi ở đây.
          </p>
        </div>
      </div>

      <form
        className="toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void load();
        }}
      >
        <Field label="Loại tài nguyên" htmlFor="resource_type">
          <Select
            value={resourceType}
            onChange={(e) =>
              setResourceType(e.target.value as AuditResourceType | "")
            }
          >
            <option value="">Tất cả</option>
            {RESOURCE_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Hành động" htmlFor="action" hint="ví dụ: seller.suspend">
          <Input
            value={action}
            onChange={(e) => setAction(e.target.value)}
            placeholder="ledger.adjust"
          />
        </Field>

        <Field label="Từ ngày" htmlFor="from">
          <Input
            type="date"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </Field>

        <Field label="Đến ngày" htmlFor="to">
          <Input
            type="date"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </Field>

        <Button type="submit" variant="primary">
          Lọc
        </Button>
      </form>

      {error && <Alert tone="danger">{error}</Alert>}

      <Table
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        empty="Không có bản ghi nào khớp bộ lọc."
      />

      {hasMore && (
        <div style={{ marginTop: "var(--space-4)" }}>
          <Button
            loading={loading}
            onClick={() => void load({ append: true, cursor })}
          >
            Tải thêm
          </Button>
        </div>
      )}
    </>
  );
}

function errorText(e: unknown): string {
  if (isApiError(e)) {
    // Nhật ký CHỈ vai trò ADMIN đọc được — nói rõ thay vì để người dùng
    // tưởng hệ thống hỏng.
    if (e.isForbidden) {
      return "Chỉ vai trò ADMIN đọc được nhật ký thao tác.";
    }
    return e.message;
  }
  return "Không kết nối được máy chủ.";
}
