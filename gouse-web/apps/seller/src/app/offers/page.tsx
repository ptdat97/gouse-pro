"use client";

import {
  createOffer,
  isApiError,
  listMyOffers,
  updateInventory,
  updateOffer,
  type MyOffers,
} from "@fc/api-client";
import { Alert, Badge, Button, Field, Input } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { money } from "@/lib/format";
import { offerStatusLabel } from "@/lib/status";
import { useSession } from "@/lib/session";

type Offer = NonNullable<MyOffers["data"]>[number];

/**
 * Hàng đang bán.
 *
 * # Giá và tồn kho là HAI việc khác nhau
 *
 * Chúng ở hai module khác nhau ở backend, và nhầm lẫn giữa chúng là nguồn
 * sai sót phổ biến: đổi giá không làm hàng nhiều lên, và kiểm kê không làm
 * hàng rẻ đi. Giao diện tách rõ hai ô nhập.
 *
 * # Kiểm kê nhận con số ĐÃ ĐẾM
 *
 * Không phải "thêm bao nhiêu". Đó là cách người đứng trong kho nghĩ, và bắt
 * họ tự trừ là mời gọi lỗi số học vào con số quyết định bán được bao nhiêu.
 */
export default function OffersPage() {
  return (
    <Shell>
      <Offers />
    </Shell>
  );
}

function Offers() {
  const { api } = useSession();
  const [rows, setRows] = React.useState<Offer[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [creating, setCreating] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listMyOffers(api);
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

  return (
    <div>
      <h1>Hàng đang bán</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      <div className="actions">
        <Button onClick={() => setCreating((v) => !v)}>
          {creating ? "Đóng" : "Đăng bán sản phẩm"}
        </Button>
      </div>

      {creating && (
        <CreateOfferForm
          onCreated={() => {
            setCreating(false);
            void load();
          }}
        />
      )}

      {rows.length === 0 ? (
        <p className="muted">Bạn chưa đăng bán sản phẩm nào.</p>
      ) : (
        rows.map((o) => <OfferCard key={o.id} offer={o} onChanged={load} />)
      )}
    </div>
  );
}

/**
 * Form đăng bán.
 *
 * `initial_inventory` KHÔNG phải tùy chọn trong thực tế: offer không có
 * hàng thì hết hàng ngay từ giây đầu, và không có đường nào để nhập sau
 * ngoài kiểm kê một bản ghi chưa tồn tại.
 */
function CreateOfferForm({ onCreated }: { onCreated: () => void }) {
  const { api } = useSession();
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true);
    setError(null);
    try {
      await createOffer(api, {
        sku_id: String(f.get("sku_id") ?? "").trim(),
        price: {
          amount: Number(f.get("price")),
          currency: "VND",
        },
        initial_inventory: { quantity: Number(f.get("quantity")) },
      });
      onCreated();
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không đăng bán được");
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <h2>Đăng bán sản phẩm</h2>
      {error && <Alert tone="danger">{error}</Alert>}

      <form onSubmit={onSubmit}>
        <Field
          label="Mã SKU"
          htmlFor="sku_id"
          hint="Mã của tổ hợp màu/size trong danh mục nền tảng"
        >
          <Input id="sku_id" name="sku_id" required />
        </Field>

        <div className="form-row">
          <Field label="Giá bán (đ)" htmlFor="price">
            <Input id="price" name="price" type="number" min={1} required />
          </Field>
          <Field
            label="Số lượng có sẵn"
            htmlFor="quantity"
            hint="Không nhập thì sản phẩm hiện hết hàng ngay"
          >
            <Input id="quantity" name="quantity" type="number" min={0} required />
          </Field>
        </div>

        <div className="actions">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Đang đăng…" : "Đăng bán"}
          </Button>
        </div>
      </form>
    </section>
  );
}

function OfferCard({ offer, onChanged }: { offer: Offer; onChanged: () => void }) {
  const { api } = useSession();
  const [error, setError] = React.useState<string | null>(null);
  const [note, setNote] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);

  async function run<T>(fn: () => Promise<T>, ok: (result: T) => string) {
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      const result = await fn();
      setNote(ok(result));
      onChanged();
    } catch (e) {
      setError(isApiError(e) ? e.message : "Thao tác không thành công");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <p>
        <strong>{money(offer.price)}</strong>{" "}
        <Badge tone={offer.status === "ACTIVE" ? "success" : "neutral"}>
          {offerStatusLabel(offer.status)}
        </Badge>
      </p>
      <p className="muted">SKU {offer.sku_id}</p>

      {error && <Alert tone="danger">{error}</Alert>}
      {note && <Alert tone="success">{note}</Alert>}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void run(
            () =>
              updateOffer(api, offer.id, {
                price: { amount: Number(f.get("price")), currency: "VND" },
              }),
            () => "Đã đổi giá.",
          );
        }}
      >
        <div className="form-row">
          <Field label="Giá mới (đ)" htmlFor={`price-${offer.id}`}>
            <Input
              id={`price-${offer.id}`}
              name="price"
              type="number"
              min={1}
              defaultValue={offer.price?.amount}
            />
          </Field>
          <div style={{ alignSelf: "end" }}>
            <Button type="submit" disabled={busy}>
              Lưu giá
            </Button>
          </div>
        </div>
      </form>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void run(
            () =>
              updateInventory(
                api,
                offer.sku_id,
                Number(f.get("quantity")),
                String(f.get("reason") ?? ""),
              ),
            // Hiện con số máy chủ TRẢ VỀ, không phải con số vừa gõ. Hai
            // con số lệch nhau khi có đơn chen vào giữa lúc ghi, và lúc
            // đó cái seller cần thấy là cái đang có thật trong kho.
            (r) => `Tồn kho hiện tại: ${r.quantity_available}.`,
          );
        }}
      >
        <div className="form-row">
          <Field
            label="Số lượng đã đếm"
            htmlFor={`qty-${offer.id}`}
            hint="Con số THỰC TẾ trong kho, không phải số cần thêm"
          >
            <Input
              id={`qty-${offer.id}`}
              name="quantity"
              type="number"
              min={0}
            />
          </Field>
          <Field
            label="Lý do"
            htmlFor={`reason-${offer.id}`}
            hint="Tối thiểu 5 ký tự — cần để đối soát khi tồn kho lệch"
          >
            <Input
              id={`reason-${offer.id}`}
              name="reason"
              minLength={5}
              placeholder="Kiểm kê thực tế"
            />
          </Field>
          <div style={{ alignSelf: "end" }}>
            <Button type="submit" disabled={busy}>
              Cập nhật kho
            </Button>
          </div>
        </div>
      </form>

      {offer.status === "ACTIVE" && (
        <div className="actions">
          <Button
            disabled={busy}
            onClick={() =>
              void run(
                () => updateOffer(api, offer.id, { status: "ARCHIVED" }),
                () => "Đã ngừng bán.",
              )
            }
          >
            Ngừng bán
          </Button>
        </div>
      )}
    </section>
  );
}
