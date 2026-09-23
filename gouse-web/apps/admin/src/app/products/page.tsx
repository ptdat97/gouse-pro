"use client";

import {
  approveProduct,
  getBrand,
  isApiError,
  listPendingProducts,
  listSellersByIds,
  rejectProduct,
  type PendingProducts,
  type SellerRef,
} from "@fc/api-client";
import { Alert, Badge, Button, ReasonDialog } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime } from "@/lib/format";
import { useSession } from "@/lib/session";

type Product = NonNullable<PendingProducts["data"]>[number];

/**
 * Duyệt sản phẩm.
 *
 * # Vì sao trang này tồn tại
 *
 * Ba endpoint duyệt có từ lâu và không màn hình nào gọi chúng. Nhà bán gửi
 * duyệt xong thì sản phẩm nằm ở `PENDING_REVIEW` vĩnh viễn — cả luồng đăng
 * bán kết thúc ở một ngõ cụt. Xem P3-68.
 *
 * # Người duyệt cần thấy ĐỦ để quyết, ngay tại đây
 *
 * Duyệt một trang sản phẩm là trả lời: ảnh có đúng hàng không, mô tả có
 * nói quá không, chất liệu có khai không, và — quan trọng nhất —
 * **gian hàng này có tư cách bán thương hiệu đó không**.
 *
 * Nên trang hiện ảnh thật, mô tả đầy đủ, biến thể, TÊN gian hàng và TÊN
 * thương hiệu. Một danh sách chỉ có tên sản phẩm và hai cái nút sẽ biến
 * việc duyệt thành bấm cho xong — và khi ấy nó tệ hơn không duyệt, vì nó
 * tạo ra một lớp bảo vệ chỉ có trên giấy.
 */
export default function ProductsPage() {
  return (
    <Shell>
      <PendingView />
    </Shell>
  );
}

function PendingView() {
  const { api } = useSession();
  const [rows, setRows] = React.useState<Product[]>([]);
  const [sellers, setSellers] = React.useState<Record<string, SellerRef>>({});
  const [brands, setBrands] = React.useState<Record<string, string>>({});
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const load = React.useCallback(async () => {
    setError(null);
    try {
      const res = await listPendingProducts(api);
      const ds = res.data ?? [];
      setRows(ds);

      // Tên gian hàng: MỘT lượt gọi cho cả trang, không phải một lượt mỗi
      // sản phẩm. `lookupSellers` nhận danh sách mã đúng vì việc này.
      const maBan = [
        ...new Set(ds.map((p) => p.created_by_seller_id).filter(Boolean)),
      ] as string[];
      if (maBan.length) {
        const ss = await listSellersByIds(api, maBan);
        setSellers(Object.fromEntries(ss.map((s) => [s.id!, s])));
      }

      // Tên thương hiệu: một lượt cho mỗi thương hiệu KHÁC NHAU. Không có
       // endpoint tra theo lô cho thương hiệu, và số thương hiệu trên một
      // trang chờ duyệt thường rất nhỏ — nên chấp nhận, thay vì hiện một
      // dãy ULID mà người duyệt không đọc được.
      const maTH = [...new Set(ds.map((p) => p.brand_id).filter(Boolean))] as string[];
      const ten: Record<string, string> = {};
      await Promise.all(
        maTH.map(async (id) => {
          try {
            const b = await getBrand(api, id);
            if (b.name) ten[id] = b.name;
          } catch {
            // Thương hiệu tra không ra KHÔNG được làm hỏng cả trang: người
            // duyệt vẫn cần xử lý những sản phẩm còn lại. Hiện mã thô.
          }
        }),
      );
      setBrands(ten);
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không tải được hàng chờ duyệt");
    } finally {
      setLoading(false);
    }
  }, [api]);

  React.useEffect(() => {
    void load();
  }, [load]);

  if (loading) return <p>Đang tải…</p>;

  return (
    <div>
      <header className="page__header">
        <h1>Duyệt sản phẩm</h1>
        <p className="page__lead">
          {rows.length === 0
            ? "Không có sản phẩm nào chờ duyệt."
            : `${rows.length} sản phẩm đang chờ. Hàng ĐANG BÁN vừa bị sửa cũng
               nằm ở đây — chúng đang tạm ẩn khỏi cửa hàng, nên để lâu là
               mất doanh số thật.`}
        </p>
      </header>

      {error && <Alert tone="danger">{error}</Alert>}

      {rows.map((p) => (
        <Card
          key={p.id}
          p={p}
          seller={p.created_by_seller_id ? sellers[p.created_by_seller_id] : undefined}
          brandName={p.brand_id ? brands[p.brand_id] : undefined}
          onDone={load}
        />
      ))}
    </div>
  );
}

function Card({
  p,
  seller,
  brandName,
  onDone,
}: {
  p: Product;
  seller: SellerRef | undefined;
  brandName: string | undefined;
  onDone: () => void;
}) {
  const { api } = useSession();
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [tuChoiMo, setTuChoiMo] = React.useState(false);

  const anh = p.images ?? [];
  const bienThe = p.variants ?? [];

  async function duyet() {
    setBusy(true);
    setError(null);
    try {
      await approveProduct(api, p.id!);
      onDone();
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không duyệt được");
    } finally {
      setBusy(false);
    }
  }

  async function tuChoi(lyDo: string) {
    setBusy(true);
    setError(null);
    try {
      await rejectProduct(api, p.id!, lyDo);
      setTuChoiMo(false);
      onDone();
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không từ chối được");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <p>
        <strong>{p.name}</strong>{" "}
        <Badge tone="info">Chờ duyệt</Badge>{" "}
        <span className="muted">{p.slug}</span>
      </p>

      {/*
        AI gửi và DƯỚI thương hiệu nào — hai câu hỏi quyết định việc duyệt.
        Đặt ngay dưới tên, không giấu trong phần chi tiết.
      */}
      <div className="detail__grid">
        <Muc nhan="Gian hàng">
          {seller?.name ?? <span className="mono">{p.created_by_seller_id || "nền tảng"}</span>}
        </Muc>
        <Muc nhan="Thương hiệu">
          {brandName ?? <span className="mono">{p.brand_id}</span>}
        </Muc>
        <Muc nhan="Loại">{p.product_type ?? "—"}</Muc>
        <Muc nhan="Gửi lúc">{dateTime(p.created_at)}</Muc>
      </div>

      {/*
        Lý do từ chối LẦN TRƯỚC, nếu có.
        Người duyệt cần biết sản phẩm này đã bị trả về vì gì, để kiểm đúng
        chỗ đó đã sửa chưa — thay vì đọc lại từ đầu.
      */}
      {p.rejection_reason && (
        <Alert tone="warning">
          Lần trước bị trả về vì: {p.rejection_reason}
        </Alert>
      )}

      <p className="muted">{p.description || "(chưa có mô tả)"}</p>

      <div className="detail__grid">
        <Muc nhan="Chất liệu">{p.material_composition || "—"}</Muc>
        <Muc nhan="Bảo quản">{p.care_instructions || "—"}</Muc>
        <Muc nhan="Xuất xứ">{p.origin_country || "—"}</Muc>
        <Muc nhan="Biến thể">
          {bienThe.length === 0
            ? "—"
            : bienThe
                .map((v) =>
                  Object.entries(v.attributes ?? {})
                    .filter(([k]) => k === "color" || k === "size")
                    .map(([, x]) => x)
                    .join(" / "),
                )
                .join(" · ")}
        </Muc>
      </div>

      {/*
        ẢNH THẬT, không phải số đếm.
        "3 ảnh" không trả lời được câu hỏi duy nhất mà người duyệt có: ảnh
        này có đúng là món hàng đang bán không.
      */}
      {anh.length > 0 && (
        <div className="anh-duyet">
          {anh.map((u) => (
            // eslint-disable-next-line @next/next/no-img-element
            <img key={u} src={u} alt="" loading="lazy" />
          ))}
        </div>
      )}

      {error && <Alert tone="danger">{error}</Alert>}

      <p className="actions">
        <Button disabled={busy} onClick={() => void duyet()}>
          Duyệt, cho lên kệ
        </Button>
        <Button variant="secondary" disabled={busy} onClick={() => setTuChoiMo(true)}>
          Trả về để sửa
        </Button>
      </p>

      {/*
        Dùng ReasonDialog dùng chung: nó BẮT lý do tối thiểu 20 ký tự.
        "sai" hay "ko dc" là một lời từ chối không ai sửa được theo.
      */}
      <ReasonDialog
        open={tuChoiMo}
        onOpenChange={setTuChoiMo}
        title={`Trả về: ${p.name}`}
        impact={
          <span>
            Sản phẩm quay về trạng thái <strong>nháp</strong> của gian hàng.
            Lý do dưới đây <strong>nhà bán đọc được nguyên văn</strong> —
            hãy nói rõ phải sửa gì.
          </span>
        }
        confirmLabel="Trả về cho nhà bán"
        confirmTone="danger"
        submitting={busy}
        serverError={error}
        onConfirm={(lyDo) => void tuChoi(lyDo)}
      />
    </section>
  );
}

function Muc({ nhan, children }: { nhan: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="detail__label">{nhan}</div>
      <div className="detail__value">{children}</div>
    </div>
  );
}
