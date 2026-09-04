"use client";

import {
  isApiError,
  listOpsConfig,
  setOpsConfig,
  type OpsConfigItem,
} from "@fc/api-client";
import { Alert, Badge, Button, ReasonDialog, Table, type Column } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime } from "@/lib/format";
import { useSession } from "@/lib/session";

/**
 * Cấu hình vận hành.
 *
 * # Trang này KHÔNG thêm được tham số mới, và đó là chủ đích
 *
 * Danh sách khóa là tập ĐÓNG khai trong mã nguồn backend. Thêm một tham số
 * là việc của người viết mã, có review — vì mỗi tham số mới là một câu hỏi
 * "sửa được lúc chạy có an toàn không", và câu hỏi đó không trả lời được
 * bằng một cái form.
 *
 * # Vì sao mỗi lần đổi đều hỏi LÝ DO
 *
 * Những con số ở đây quyết định cách hệ thống chấm điểm nhà bán. Hạ hạn
 * giao hàng làm hàng loạt gian hàng đột ngột bị chấm là giao trễ, và điểm
 * đó ảnh hưởng tới việc họ thắng buy box.
 *
 * Nói cách khác: đổi một con số ở đây làm thay đổi thu nhập của người
 * ngoài công ty. Một lần đổi không có người chịu trách nhiệm và không có
 * lý do thì không giải thích được khi họ khiếu nại.
 */
export default function ConfigPage() {
  return (
    <Shell>
      <ConfigTable />
    </Shell>
  );
}

/** Đổi giá trị lưu trữ thành chuỗi người đọc được. */
function hienThi(item: OpsConfigItem): string {
  const v = item.value ?? 0;
  switch (item.type) {
    case "duration":
      return `${v} giờ`;
    case "ratio":
      // Tỷ lệ hiện bằng phần trăm: "0.03" khó đọc hơn "3%", và người đọc
      // sai một chữ số thập phân ở đây là đổi ngưỡng gấp mười lần.
      return `${(v * 100).toFixed(1)}%`;
    default:
      return String(v);
  }
}

/** Đơn vị người dùng NHẬP — phải khớp với cách hiển thị. */
function donVi(item: OpsConfigItem): string {
  switch (item.type) {
    case "duration":
      return "giờ";
    case "ratio":
      return "phần trăm (%)";
    default:
      return "";
  }
}

/** Đổi số người nhập thành giá trị gửi lên API. */
function veGiaTriApi(item: OpsConfigItem, nhap: number): number {
  return item.type === "ratio" ? nhap / 100 : nhap;
}

function veGiaTriNhap(item: OpsConfigItem): number {
  const v = item.value ?? 0;
  return item.type === "ratio" ? v * 100 : v;
}

function ConfigTable() {
  const { api } = useSession();
  const [rows, setRows] = React.useState<OpsConfigItem[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const [dangSua, setDangSua] = React.useState<OpsConfigItem | null>(null);
  const [giaTriMoi, setGiaTriMoi] = React.useState("");
  const [luuError, setLuuError] = React.useState<string | null>(null);
  const [dangLuu, setDangLuu] = React.useState(false);

  const tai = React.useCallback(() => {
    setLoading(true);
    listOpsConfig(api)
      .then((r) => {
        setRows(r.data ?? []);
        setError(null);
      })
      .catch((e) =>
        setError(isApiError(e) ? e.message : "Không tải được cấu hình"),
      )
      .finally(() => setLoading(false));
  }, [api]);

  React.useEffect(tai, [tai]);

  function moSua(item: OpsConfigItem) {
    setDangSua(item);
    setGiaTriMoi(String(veGiaTriNhap(item)));
    setLuuError(null);
  }

  function luu(lyDo: string) {
    if (!dangSua) return;

    const nhap = Number(giaTriMoi);
    if (!Number.isFinite(nhap)) {
      setLuuError("Giá trị phải là một con số");
      return;
    }

    setDangLuu(true);
    setLuuError(null);
    setOpsConfig(api, dangSua.key ?? "", veGiaTriApi(dangSua, nhap), lyDo)
      .then(() => {
        setDangSua(null);
        tai();
      })
      .catch((e) =>
        // Giữ hộp thoại MỞ khi lỗi: đóng nó làm mất lý do vừa gõ, và
        // người dùng phải gõ lại từ đầu chỉ vì nhập sai một con số.
        setLuuError(isApiError(e) ? e.message : "Không lưu được"),
      )
      .finally(() => setDangLuu(false));
  }

  const cols: Column<OpsConfigItem>[] = [
    {
      header: "Tham số",
      cell: (r: OpsConfigItem) => (
        <div>
          <code>{r.key}</code>
          <p className="muted">{r.description}</p>
        </div>
      ),
    },
    {
      header: "Giá trị",
      cell: (r: OpsConfigItem) => (
        <div>
          <strong>{hienThi(r)}</strong>{" "}
          {r.is_default ? (
            <Badge tone="neutral">mặc định</Badge>
          ) : (
            <Badge tone="info">đã đổi</Badge>
          )}
        </div>
      ),
    },
    {
      header: "Cho phép",
      cell: (r: OpsConfigItem) =>
        r.type === "ratio"
          ? `${((r.min ?? 0) * 100).toFixed(0)}–${((r.max ?? 0) * 100).toFixed(0)}%`
          : `${r.min}–${r.max}`,
    },
    {
      header: "Lần đổi gần nhất",
      cell: (r: OpsConfigItem) =>
        r.updated_at ? (
          <div>
            <div>{dateTime(r.updated_at)}</div>
            <p className="muted">{r.reason}</p>
          </div>
        ) : (
          <span className="muted">chưa đổi lần nào</span>
        ),
    },
    {
      header: "",
      cell: (r: OpsConfigItem) => (
        <Button onClick={() => moSua(r)} variant="secondary">
          Đổi
        </Button>
      ),
    },
  ];

  return (
    <section>
      <h1>Cấu hình vận hành</h1>
      <p className="muted">
        Những con số này quyết định cách hệ thống chấm điểm nhà bán. Mọi lần
        đổi đều được ghi vào nhật ký thao tác kèm lý do, giá trị cũ và giá
        trị mới.
      </p>

      {error && <Alert tone="danger">{error}</Alert>}

      {loading ? (
        <p className="muted">Đang tải…</p>
      ) : (
        <Table
          columns={cols}
          rows={rows}
          empty="Không có tham số nào"
          rowKey={(r) => r.key ?? ""}
        />
      )}

      <ReasonDialog
        open={dangSua !== null}
        onOpenChange={(o) => !o && setDangSua(null)}
        title={`Đổi ${dangSua?.key ?? ""}`}
        // Hệ quả hiện TRƯỚC khi bấm. Người đổi con số hiếm khi là người
        // viết đoạn mã đọc nó, và "48" không tự nói rằng hạ nó xuống sẽ
        // làm hàng loạt gian hàng đột ngột bị chấm là giao trễ.
        impact={dangSua?.impact}
        warning={
          <>
            Áp dụng NGAY cho mọi lượt tính sau đó, kể cả những đơn đã hoàn
            tất từ trước.
          </>
        }
        confirmLabel="Đổi tham số"
        confirmTone="danger"
        serverError={luuError}
        submitting={dangLuu}
        onConfirm={luu}
      >
        <label>
          Giá trị mới{dangSua ? ` (${donVi(dangSua)})` : ""}
          <input
            type="number"
            step="any"
            value={giaTriMoi}
            onChange={(e) => setGiaTriMoi(e.target.value)}
          />
        </label>
        {dangSua && (
          <p className="muted">
            Hiện tại: {hienThi(dangSua)} · mặc định:{" "}
            {dangSua.type === "ratio"
              ? `${((dangSua.default ?? 0) * 100).toFixed(1)}%`
              : dangSua.default}
          </p>
        )}
      </ReasonDialog>
    </section>
  );
}
