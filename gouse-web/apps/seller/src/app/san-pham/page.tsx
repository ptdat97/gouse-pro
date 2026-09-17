"use client";

import {
  createMyProduct,
  getCategoryTree,
  isApiError,
  listBrandsIMaySell,
  listMyProducts,
  submitMyProduct,
  type BrandsIMaySell,
  type CategoryTree,
  type MyProducts,
} from "@fc/api-client";
import { Alert, Badge, Button, Field, Input, Select, Textarea } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime } from "@/lib/format";
import {
  GIOI_TINH,
  LOAI_SAN_PHAM,
  productStatusLabel,
  productTone,
  slugTu,
} from "@/lib/san-pham";
import { useSession } from "@/lib/session";

type Product = NonNullable<MyProducts["data"]>[number];
type Brand = NonNullable<BrandsIMaySell["data"]>[number];
type Cat = NonNullable<CategoryTree["data"]>[number];

/**
 * Sản phẩm của tôi — xem, tạo nháp, gửi duyệt.
 *
 * # Vòng đời quyết định bố cục
 *
 *	DRAFT           chỉ mình thấy. Sửa thoải mái, thiếu gì cũng được.
 *	PENDING_REVIEW  đang chờ người kiểm duyệt. Không sửa được.
 *	ACTIVE          khách thấy.
 *	INACTIVE        tạm ngừng bán.
 *	ARCHIVED        đã lưu trữ.
 *
 * KHÔNG có `REJECTED`: bị từ chối đưa sản phẩm VỀ LẠI `DRAFT` kèm
 * `rejection_reason`. Nên "nháp mới viết" và "nháp vừa bị trả về" trông
 * khác nhau trên màn hình dù cùng một `status` — và LÝ DO TRẢ VỀ là thứ
 * quan trọng nhất trang này mang.
 *
 * # Điều kiện "đủ thông tin" bật ở GỬI DUYỆT, không ở lúc tạo
 *
 * Tạo nháp thiếu ảnh, thiếu mô tả, thiếu biến thể đều được — đó là cách
 * người ta làm việc thật. Gửi duyệt thì không, và backend nói rõ thiếu gì.
 * Biểu mẫu ở đây vì thế chỉ đòi những trường mà CHÍNH lượt tạo cần.
 */
export default function ProductsPage() {
  return (
    <Shell>
      <Products />
    </Shell>
  );
}

function Products() {
  const { api } = useSession();
  const [rows, setRows] = React.useState<Product[]>([]);
  const [brands, setBrands] = React.useState<Brand[]>([]);
  const [cats, setCats] = React.useState<Cat[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [dangTao, setDangTao] = React.useState(false);

  const load = React.useCallback(async () => {
    setError(null);
    try {
      const [sp, th, dm] = await Promise.all([
        listMyProducts(api),
        listBrandsIMaySell(api),
        getCategoryTree(api),
      ]);
      setRows(sp.data ?? []);
      setBrands(th.data ?? []);
      setCats(dm.data ?? []);
    } catch (e) {
      setError(isApiError(e) ? e.message : "Không tải được danh sách sản phẩm");
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
      <h1>Sản phẩm của tôi</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      {!dangTao && (
        <p>
          <Button onClick={() => setDangTao(true)} disabled={brands.length === 0}>
            Tạo sản phẩm mới
          </Button>
        </p>
      )}

      {/*
        KHÔNG có thương hiệu nào thì nói vì sao, thay vì hiện một nút bấm
        không được. Đây là hàng rào chống hàng giả đang làm việc, không
        phải hệ thống hỏng — và nhà bán cần biết phải làm gì tiếp.
      */}
      {brands.length === 0 && (
        <Alert tone="warning">
          Gian hàng của bạn chưa được phép đăng bán dưới thương hiệu nào.
          Liên hệ nền tảng để được cấp quyền cho thương hiệu bạn phân phối.
        </Alert>
      )}

      {dangTao && (
        <TaoSanPham
          brands={brands}
          cats={cats}
          onXong={() => {
            setDangTao(false);
            void load();
          }}
          onHuy={() => setDangTao(false)}
        />
      )}

      <DanhSach rows={rows} onDoi={load} />
    </div>
  );
}

function TaoSanPham({
  brands,
  cats,
  onXong,
  onHuy,
}: {
  brands: Brand[];
  cats: Cat[];
  onXong: () => void;
  onHuy: () => void;
}) {
  const { api } = useSession();
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const [brandID, setBrandID] = React.useState(brands[0]?.id ?? "");
  const [catID, setCatID] = React.useState("");
  const [name, setName] = React.useState("");
  const [slug, setSlug] = React.useState("");
  const [slugTuTay, setSlugTuTay] = React.useState(false);
  const [loai, setLoai] = React.useState("TOP");
  const [gioi, setGioi] = React.useState("WOMEN");
  const [moTa, setMoTa] = React.useState("");
  const [chatLieu, setChatLieu] = React.useState("");

  const phang = React.useMemo(() => lamPhang(cats), [cats]);

  async function gui(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await createMyProduct(api, {
        brand_id: brandID,
        category_id: catID,
        name: name.trim(),
        slug: slug.trim(),
        product_type: loai,
        gender_target: gioi,
        description: moTa.trim() || undefined,
        material_composition: chatLieu.trim() || undefined,
      });
      onXong();
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không tạo được sản phẩm");
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <h2>Sản phẩm mới</h2>
      <p className="muted">
        Sản phẩm sinh ra ở trạng thái <strong>Nháp</strong> — chưa ai thấy.
        Ảnh, biến thể và bảng size bổ sung sau; chúng chỉ bắt buộc khi gửi
        duyệt.
      </p>

      {error && <Alert tone="danger">{error}</Alert>}

      <form onSubmit={gui}>
        <Field label="Thương hiệu" htmlFor="th">
          <Select
            id="th"
            value={brandID}
            onChange={(e) => setBrandID(e.target.value)}
            required
          >
            {brands.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
                {b.protection_level === "RESTRICTED" ? " (của bạn)" : ""}
              </option>
            ))}
          </Select>
        </Field>

        {/*
          Danh mục BẮT BUỘC, và ô này có `required`.
          Miền đòi danh mục; tầng HTTP từng coi nó là tùy chọn và trả 500
          khi thiếu. Chặn ở đây là để nhà bán không bao giờ chạm vào lỗi
          ấy — nhưng backend vẫn kiểm lại, vì giao diện không phải hàng rào.
        */}
        <Field label="Danh mục" htmlFor="dm">
          <Select
            id="dm"
            value={catID}
            onChange={(e) => setCatID(e.target.value)}
            required
          >
            <option value="">— chọn danh mục —</option>
            {phang.map((c) => (
              <option key={c.id} value={c.id}>
                {c.nhan}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Tên sản phẩm" htmlFor="ten">
          <Input
            id="ten"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              // Slug tự sinh cho tới khi người dùng tự sửa. Bắt gõ tay một
              // chuỗi không dấu là bước dễ bỏ nhất của cả biểu mẫu.
              if (!slugTuTay) setSlug(slugTu(e.target.value));
            }}
            required
          />
        </Field>

        <Field
          label="Đường dẫn (slug)"
          htmlFor="slug"
          hint="Nằm trong địa chỉ trang sản phẩm. Phải khác mọi sản phẩm đã có."
        >
          <Input
            id="slug"
            value={slug}
            onChange={(e) => {
              setSlugTuTay(true);
              setSlug(e.target.value);
            }}
            required
          />
        </Field>

        <Field label="Loại" htmlFor="loai">
          <Select id="loai" value={loai} onChange={(e) => setLoai(e.target.value)}>
            {LOAI_SAN_PHAM.map((x) => (
              <option key={x.ma} value={x.ma}>
                {x.nhan}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Dành cho" htmlFor="gioi">
          <Select id="gioi" value={gioi} onChange={(e) => setGioi(e.target.value)}>
            {GIOI_TINH.map((x) => (
              <option key={x.ma} value={x.ma}>
                {x.nhan}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Mô tả" htmlFor="mota">
          <Textarea
            id="mota"
            value={moTa}
            onChange={(e) => setMoTa(e.target.value)}
            rows={3}
          />
        </Field>

        <Field
          label="Thành phần chất liệu"
          htmlFor="cl"
          hint="Ví dụ: 80% cotton, 20% polyester. Bắt buộc khi gửi duyệt."
        >
          <Input
            id="cl"
            value={chatLieu}
            onChange={(e) => setChatLieu(e.target.value)}
          />
        </Field>

        <p className="actions">
          <Button type="submit" disabled={busy}>
            Tạo nháp
          </Button>
          <Button type="button" variant="secondary" disabled={busy} onClick={onHuy}>
            Hủy
          </Button>
        </p>
      </form>
    </section>
  );
}

/** Cây danh mục → danh sách phẳng có thụt lề, giữ quan hệ cha–con. */
function lamPhang(cats: Cat[], sau = 0): { id: string; nhan: string }[] {
  const ra: { id: string; nhan: string }[] = [];
  for (const c of cats) {
    ra.push({ id: c.id!, nhan: `${"— ".repeat(sau)}${c.name}` });
    const con = (c.children ?? []) as Cat[];
    if (con.length) ra.push(...lamPhang(con, sau + 1));
  }
  return ra;
}

function DanhSach({ rows, onDoi }: { rows: Product[]; onDoi: () => void }) {
  if (rows.length === 0) {
    return <p className="muted">Chưa có sản phẩm nào.</p>;
  }
  return (
    <section>
      <h2>Đã có</h2>
      {rows.map((p) => (
        <Dong key={p.id} p={p} onDoi={onDoi} />
      ))}
    </section>
  );
}

function Dong({ p, onDoi }: { p: Product; onDoi: () => void }) {
  const { api } = useSession();
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  async function guiDuyet() {
    setBusy(true);
    setError(null);
    try {
      await submitMyProduct(api, p.id!);
      onDoi();
    } catch (e) {
      // Backend nói THIẾU GÌ ("Sản phẩm chưa có ảnh nào", "chưa có biến
      // thể nào"). Hiện nguyên văn: đó là danh sách việc cần làm.
      setError(isApiError(e) ? e.message : "Không gửi duyệt được");
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <p>
        <strong>{p.name}</strong>{" "}
        <Badge tone={productTone(p.status, p.rejection_reason)}>
          {productStatusLabel(p.status, p.rejection_reason)}
        </Badge>{" "}
        <span className="muted">{p.slug}</span>
      </p>
      <p className="muted">Tạo lúc {dateTime(p.created_at)}</p>

      {/*
        LÝ DO TỪ CHỐI là thông tin quan trọng nhất trang này mang.
        Không có nó, nhà bán chỉ biết mình bị từ chối và sẽ gửi lại y hệt.
      */}
      {p.rejection_reason && (
        <Alert tone="danger">
          Bị trả về: {p.rejection_reason}
        </Alert>
      )}

      {error && <Alert tone="danger">{error}</Alert>}

      {p.status === "DRAFT" && (
        <p>
          <Button disabled={busy} onClick={() => void guiDuyet()}>
            Gửi duyệt
          </Button>
          <span className="muted">
            {" "}
            Cần đủ mô tả, ảnh, biến thể và bảng size.
          </span>
        </p>
      )}
    </section>
  );
}
