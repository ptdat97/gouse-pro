"use client";

import {
  approveSeller,
  getSeller,
  isApiError,
  listSellers,
  suspendSeller,
  type SellerDetail,
  type SellerStatus,
} from "@fc/api-client";
import {
  Alert,
  Badge,
  Button,
  Field,
  Input,
  ReasonDialog,
  Select,
  Table,
  type Column,
} from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { basisPoints, dateTime } from "@/lib/format";
import { sellerTone } from "@/lib/status";
import { useSession } from "@/lib/session";

/**
 * Trang quản trị nhà bán.
 *
 * Nguyên tắc từ admin.md mục 8: **ưu tiên "cần xử lý ngay"**. Nhân viên vào
 * đây để làm việc, nên bộ lọc mặc định là hồ sơ CHỜ DUYỆT, không phải toàn
 * bộ danh sách.
 */

type Row = SellerDetail;

export default function SellersPage() {
  return (
    <Shell>
      <SellersView />
    </Shell>
  );
}

function SellersView() {
  const { api } = useSession();

  // Mặc định là hàng đợi việc phải làm, không phải "tất cả".
  const [status, setStatus] = React.useState<SellerStatus | "">(
    "PENDING_REVIEW" as SellerStatus,
  );
  const [rows, setRows] = React.useState<Row[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [selected, setSelected] = React.useState<Row | null>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listSellers(api, {
        status: status === "" ? undefined : status,
        limit: 50,
      });
      setRows((res.data ?? []) as Row[]);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setLoading(false);
    }
  }, [api, status]);

  React.useEffect(() => {
    void load();
  }, [load]);

  const columns: Column<Row>[] = [
    {
      header: "Nhà bán",
      cell: (r) => (
        <>
          <div style={{ fontWeight: "var(--weight-medium)" }}>{r.name}</div>
          <div className="mono" style={{ color: "var(--color-text-muted)" }}>
            {r.slug}
          </div>
        </>
      ),
    },
    { header: "Loại", cell: (r) => r.seller_type },
    {
      header: "Trạng thái",
      cell: (r) => <Badge tone={sellerTone(r.status)}>{r.status}</Badge>,
    },
    {
      header: "Hoa hồng",
      numeric: true,
      cell: (r) => basisPoints(r.commission_rate_bp),
    },
    { header: "Ngày tạo", cell: (r) => dateTime(r.created_at) },
  ];

  return (
    <>
      <div className="page__header">
        <div>
          <h1>Nhà bán</h1>
          <p className="page__lead">
            Duyệt hồ sơ và quản lý trạng thái gian hàng.
          </p>
        </div>
      </div>

      <div className="toolbar">
        <Field label="Trạng thái" htmlFor="status">
          <Select
            value={status}
            onChange={(e) => setStatus(e.target.value as SellerStatus | "")}
          >
            <option value="PENDING_REVIEW">Chờ duyệt</option>
            <option value="APPLIED">Mới nộp</option>
            <option value="APPROVED">Đã duyệt</option>
            <option value="ACTIVE">Đang bán</option>
            <option value="SUSPENDED">Bị đình chỉ</option>
            <option value="">Tất cả</option>
          </Select>
        </Field>
        <Button onClick={() => void load()}>Tải lại</Button>
      </div>

      {error && <Alert tone="danger">{error}</Alert>}

      {loading ? (
        <p>Đang tải…</p>
      ) : (
        <Table
          columns={columns}
          rows={rows}
          rowKey={(r) => r.id}
          onRowClick={(r) => setSelected(r)}
          empty={
            status === ("PENDING_REVIEW" as SellerStatus)
              ? "Không có hồ sơ nào chờ duyệt."
              : "Không có nhà bán nào khớp bộ lọc."
          }
        />
      )}

      {selected && (
        <SellerPanel
          sellerId={selected.id}
          onClose={() => setSelected(null)}
          onChanged={() => {
            setSelected(null);
            void load();
          }}
        />
      )}
    </>
  );
}

/** Bảng chi tiết + hai thao tác: duyệt và đình chỉ. */
function SellerPanel({
  sellerId,
  onClose,
  onChanged,
}: {
  sellerId: string;
  onClose: () => void;
  onChanged: () => void;
}) {
  const { api } = useSession();
  const [seller, setSeller] = React.useState<SellerDetail | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [effects, setEffects] = React.useState<string[] | null>(null);

  const [suspendOpen, setSuspendOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [dialogError, setDialogError] = React.useState<string | null>(null);

  const [rate, setRate] = React.useState("1000");

  const load = React.useCallback(async () => {
    try {
      setSeller(await getSeller(api, sellerId));
    } catch (e) {
      setError(errorText(e));
    }
  }, [api, sellerId]);

  React.useEffect(() => {
    void load();
  }, [load]);

  if (error) return <Alert tone="danger">{error}</Alert>;
  if (!seller) return <p>Đang tải hồ sơ…</p>;

  const canApprove =
    seller.status === "PENDING_REVIEW" || seller.status === "APPLIED";
  const canSuspend = seller.status === "ACTIVE" || seller.status === "APPROVED";

  async function onApprove() {
    setSubmitting(true);
    setError(null);
    try {
      const res = await approveSeller(api, sellerId, {
        commission_rate_bp: Number(rate),
      });
      setEffects(res.side_effects ?? []);
      await load();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSubmitting(false);
    }
  }

  async function onSuspend(reason: string) {
    setSubmitting(true);
    setDialogError(null);
    try {
      await suspendSeller(api, sellerId, {
        reason,
        reason_code: "PERFORMANCE_VIOLATION",
      });
      setSuspendOpen(false);
      onChanged();
    } catch (e) {
      // Giữ hộp thoại mở và giữ nguyên lý do đã nhập — đóng nó đi nghĩa là
      // người dùng phải gõ lại toàn bộ.
      setDialogError(errorText(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="detail" style={{ marginTop: "var(--space-6)" }}>
      <div className="page__header">
        <h2>{seller.name}</h2>
        <Button onClick={onClose}>Đóng</Button>
      </div>

      {effects && (
        <Alert tone="success" title="Đã duyệt">
          <ul style={{ margin: "var(--space-2) 0 0", paddingLeft: "1.2em" }}>
            {effects.map((e) => (
              <li key={e}>{e}</li>
            ))}
          </ul>
        </Alert>
      )}

      <div className="detail__grid">
        <Item label="Trạng thái">
          <Badge tone={sellerTone(seller.status)}>{seller.status}</Badge>
        </Item>
        <Item label="Loại">{seller.seller_type}</Item>
        <Item label="Tên pháp lý">{seller.legal_name || "—"}</Item>
        <Item label="Mã số thuế">{seller.tax_code || "—"}</Item>
        <Item label="Email">{seller.email || "—"}</Item>
        <Item label="Điện thoại">{seller.phone || "—"}</Item>
        <Item label="Hoa hồng">{basisPoints(seller.commission_rate_bp)}</Item>
        <Item label="Tài khoản ngân hàng">
          {seller.bank_account_verified ? (
            <Badge tone="success">Đã xác minh</Badge>
          ) : (
            <Badge tone="warning">CHƯA xác minh</Badge>
          )}
        </Item>
        {seller.approved_by && (
          <Item label="Người duyệt">
            <span className="mono">{seller.approved_by}</span>
          </Item>
        )}
        {seller.suspension_reason && (
          <Item label="Lý do đình chỉ">{seller.suspension_reason}</Item>
        )}
      </div>

      {/*
        Xác minh tài khoản ngân hàng là bước bắt buộc trước khi seller bán
        được hàng — nêu rõ ở đây thay vì để người duyệt tự phát hiện sau.
      */}
      {!seller.bank_account_verified && seller.seller_type !== "INTERNAL" && (
        <Alert tone="warning" title="Chưa xác minh tài khoản ngân hàng">
          Duyệt hồ sơ sẽ chuyển sang APPROVED nhưng seller **chưa bán được
          hàng**. Sai tài khoản nghĩa là chuyển tiền nhầm người, rất khó thu
          hồi.
        </Alert>
      )}

      <div className="actions" style={{ marginTop: "var(--space-6)" }}>
        {canApprove && (
          <>
            <Field label="Tỷ lệ hoa hồng (phần vạn)" htmlFor="rate">
              <Input
                type="number"
                min={0}
                max={10000}
                value={rate}
                onChange={(e) => setRate(e.target.value)}
              />
            </Field>
            <Button
              variant="primary"
              loading={submitting}
              onClick={() => void onApprove()}
            >
              Duyệt hồ sơ
            </Button>
          </>
        )}

        {canSuspend && (
          <Button variant="danger" onClick={() => setSuspendOpen(true)}>
            Đình chỉ
          </Button>
        )}
      </div>

      <ReasonDialog
        open={suspendOpen}
        onOpenChange={setSuspendOpen}
        title={`Đình chỉ ${seller.name}`}
        warning="Toàn bộ offer của nhà bán này sẽ bị ẩn khỏi sàn."
        impact={
          <span>
            Đơn đang xử lý <strong>KHÔNG bị hủy</strong> — seller phải hoàn
            tất, hoặc chuyển admin xử lý kèm hoàn tiền khách.
          </span>
        }
        confirmLabel="Xác nhận đình chỉ"
        submitting={submitting}
        serverError={dialogError}
        onConfirm={(reason) => void onSuspend(reason)}
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

/** Chuyển lỗi thành câu người đọc được, không lộ chi tiết nội bộ. */
function errorText(e: unknown): string {
  if (isApiError(e)) {
    if (e.isForbidden) return "Bạn không có quyền thực hiện thao tác này.";
    return e.message;
  }
  return "Không kết nối được máy chủ.";
}
