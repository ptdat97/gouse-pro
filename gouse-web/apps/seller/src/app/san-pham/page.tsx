"use client";

import {
  addMyProductVariant,
  createMyProduct,
  getCategoryTree,
  isApiError,
  listBrandsIMaySell,
  listMyProducts,
  submitMyProduct,
  updateMyProduct,
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
  KHOA_MAU,
  KHOA_SIZE,
  LOAI_SAN_PHAM,
  maSKUGoiY,
  moTaBienThe,
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
  const [anh, setAnh] = React.useState("");

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
        images: anh
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean),
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

        {/*
          Ảnh KHÔNG bắt buộc lúc tạo.

          Từ 17 tới 19/09/2026 ô này có `required`, vì khi ấy KHÔNG có
          đường sửa sản phẩm: tạo thiếu ảnh là tạo một bản ghi chết vĩnh
          viễn. Nay có PATCH (P3-65), nên nháp thiếu ảnh bổ sung sau được —
          đúng như miền vốn cho phép.
        */}
        <Field
          label="Ảnh sản phẩm"
          htmlFor="anh"
          hint="Mỗi dòng một địa chỉ ảnh; ảnh đầu là ảnh bìa. Bổ sung sau được."
        >
          <Textarea
            id="anh"
            value={anh}
            onChange={(e) => setAnh(e.target.value)}
            rows={3}
            placeholder="https://cdn.example.com/anh-1.jpg"
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

      {(p.status === "DRAFT" || p.status === "ACTIVE") && (
        <SuaNhap p={p} onDoi={onDoi} />
      )}

      <BienThe p={p} onDoi={onDoi} />

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

/**
 * Biến thể của một sản phẩm — xem cái đã có, thêm cái mới.
 *
 * # Ô CỐ ĐỊNH, không phải cặp khóa–giá trị tự do
 *
 * `variant.go` nói vì sao: nếu mỗi gian hàng tự đặt tên khóa ("color",
 * "colour", "mau_sac") thì bộ lọc theo màu của cả danh mục vô dụng. Biểu
 * mẫu cho hai ô Màu và Size, nên chuẩn ấy được cưỡng chế bằng HÌNH DẠNG
 * giao diện chứ không bằng một dòng hướng dẫn ai cũng bỏ qua.
 *
 * # Chỉ thêm ở DRAFT
 *
 * Sản phẩm đang chờ duyệt hoặc đang bán thì đổi cấu trúc hàng là đổi thứ
 * người kiểm duyệt đã xem, hoặc thứ khách đang nhìn. Backend từ chối;
 * giao diện không mời.
 */
function BienThe({ p, onDoi }: { p: Product; onDoi: () => void }) {
  const { api } = useSession();
  const vs = p.variants ?? [];
  const [mo, setMo] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const [mau, setMau] = React.useState("");
  const [size, setSize] = React.useState("");
  const [ma, setMa] = React.useState("");
  const [maTuTay, setMaTuTay] = React.useState(false);

  function datGoiY(m: string, s: string) {
    if (!maTuTay) setMa(maSKUGoiY(p.slug ?? "", m, s));
  }

  async function them(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await addMyProductVariant(api, p.id!, {
        attributes: { [KHOA_MAU]: mau.trim(), [KHOA_SIZE]: size.trim() },
        skus: [{ sku_code: ma.trim() }],
      });
      setMau("");
      setSize("");
      setMa("");
      setMaTuTay(false);
      setMo(false);
      onDoi();
    } catch (err) {
      // Backend từ chối tổ hợp TRÙNG (409) và mã SKU đã có người dùng.
      // Hiện nguyên văn: cả hai đều là việc nhà bán sửa được ngay.
      setError(isApiError(err) ? err.message : "Không thêm được biến thể");
    } finally {
      // `finally`, KHÔNG chỉ ở nhánh lỗi.
      //
      // Bản đầu chỉ đặt lại `busy` khi thất bại. Component này KHÔNG bị
      // tháo sau một lần thêm thành công — danh sách chỉ render lại — nên
      // `busy` đứng mãi ở `true` và nút "Thêm" tắt vĩnh viễn. Nhà bán thêm
      // được ĐÚNG MỘT biến thể mỗi lần tải trang.
      //
      // Trình duyệt bắt được, TypeScript thì không: một biến trạng thái
      // không bao giờ trở về là chuyện đúng kiểu.
      setBusy(false);
    }
  }

  return (
    <div>
      {vs.length === 0 ? (
        <p className="muted">
          Chưa có biến thể nào — sản phẩm chưa gửi duyệt được.
        </p>
      ) : (
        <ul className="lines">
          {vs.map((v) => (
            <li key={v.id} className="line">
              <div>{moTaBienThe(v.attributes)}</div>
              <div className="muted">
                {(v.skus ?? []).map((s) => s.sku_code).join(" · ") || "—"}
              </div>
            </li>
          ))}
        </ul>
      )}

      {p.status === "DRAFT" && !mo && (
        <p>
          <Button variant="secondary" onClick={() => setMo(true)}>
            Thêm biến thể
          </Button>
        </p>
      )}

      {p.status === "DRAFT" && mo && (
        <form onSubmit={them}>
          {error && <Alert tone="danger">{error}</Alert>}

          <Field label="Màu" htmlFor={`mau-${p.id}`}>
            <Input
              id={`mau-${p.id}`}
              value={mau}
              onChange={(e) => {
                setMau(e.target.value);
                datGoiY(e.target.value, size);
              }}
              placeholder="Đen"
              required
            />
          </Field>

          <Field
            label="Size"
            htmlFor={`size-${p.id}`}
            hint="Theo bảng size của sản phẩm: S/M/L, 38/39/40…"
          >
            <Input
              id={`size-${p.id}`}
              value={size}
              onChange={(e) => {
                setSize(e.target.value);
                datGoiY(mau, e.target.value);
              }}
              placeholder="M"
              required
            />
          </Field>

          <Field
            label="Mã SKU"
            htmlFor={`sku-${p.id}`}
            hint="Mã kho đọc được. Phải khác mọi mã đã có trên hệ thống."
          >
            <Input
              id={`sku-${p.id}`}
              value={ma}
              onChange={(e) => {
                setMaTuTay(true);
                setMa(e.target.value);
              }}
              required
            />
          </Field>

          <p className="actions">
            <Button type="submit" disabled={busy}>
              Thêm
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={busy}
              onClick={() => setMo(false)}
            >
              Hủy
            </Button>
          </p>
        </form>
      )}
    </div>
  );
}

/**
 * Sửa sản phẩm — nháp, hoặc hàng ĐANG BÁN.
 *
 * # Điền sẵn giá trị hiện tại, gửi đi CHỈ những gì đã đổi
 *
 * Điền sẵn để nhà bán sửa một chữ không phải gõ lại cả mô tả. Chỉ gửi phần
 * đã đổi vì đó là nghĩa của PATCH: trường vắng thì máy chủ giữ nguyên. Gửi
 * lại mọi ô thì đúng hôm nay, và sai vào ngày có hai người cùng sửa — người
 * sau ghi đè phần người trước vừa đổi mà mình không hề chạm vào.
 *
 * # Hàng đang bán: nói TRƯỚC hậu quả, không để họ phát hiện sau
 *
 * Sửa hàng đang bán đưa nó về hàng chờ duyệt, tức TẠM ẨN khỏi cửa hàng.
 * Một nhà bán sửa một dấu phẩy rồi thấy hàng biến mất mà không được báo
 * trước sẽ nghĩ hệ thống hỏng — và lần sau họ không dám sửa gì nữa.
 *
 * # Thương hiệu KHÔNG có ở đây
 *
 * Không phải quên: thương hiệu được kiểm quyền bán một lần lúc tạo, và cho
 * đổi ở đây là cửa sau qua hàng rào chống hàng giả. Máy chủ từ chối; giao
 * diện không mời.
 */
function SuaNhap({ p, onDoi }: { p: Product; onDoi: () => void }) {
  const { api } = useSession();
  const [mo, setMo] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const goc = React.useMemo(
    () => ({
      name: p.name ?? "",
      description: p.description ?? "",
      material_composition: p.material_composition ?? "",
      care_instructions: p.care_instructions ?? "",
      origin_country: p.origin_country ?? "",
      images: (p.images ?? []).join("\n"),
    }),
    [p],
  );
  const [f, setF] = React.useState(goc);

  // Mở lại biểu mẫu thì bắt đầu từ giá trị MỚI NHẤT, không phải giá trị
  // của lần mở trước — danh sách đã tải lại sau mỗi lần lưu.
  React.useEffect(() => {
    if (!mo) setF(goc);
  }, [goc, mo]);

  function doi(k: keyof typeof goc) {
    return (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setF({ ...f, [k]: e.target.value });
  }

  async function luu(e: React.FormEvent) {
    e.preventDefault();

    const than: Record<string, unknown> = {};
    for (const k of [
      "name",
      "description",
      "material_composition",
      "care_instructions",
      "origin_country",
    ] as const) {
      if (f[k].trim() !== goc[k].trim()) than[k] = f[k].trim();
    }
    if (f.images !== goc.images) {
      than.images = f.images
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean);
    }
    if (Object.keys(than).length === 0) {
      setMo(false);
      return;
    }

    setBusy(true);
    setError(null);
    try {
      await updateMyProduct(api, p.id!, than);
      setMo(false);
      onDoi();
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không lưu được");
    } finally {
      setBusy(false);
    }
  }

  const dangBan = p.status === "ACTIVE";

  if (!mo) {
    return (
      <p>
        <Button variant="secondary" onClick={() => setMo(true)}>
          Sửa thông tin
        </Button>
        {dangBan && (
          <span className="muted">
            {" "}
            Sửa hàng đang bán sẽ đưa nó về hàng chờ duyệt.
          </span>
        )}
      </p>
    );
  }

  return (
    <form onSubmit={luu}>
      {error && <Alert tone="danger">{error}</Alert>}

      {/*
        Cảnh báo đặt TRONG biểu mẫu, ngay trên các ô — không phải ở hộp
        xác nhận sau khi bấm Lưu. Người ta đọc trước khi gõ, không đọc
        trước khi bấm.
      */}
      {dangBan && (
        <Alert tone="warning">
          Sản phẩm này <strong>đang bán</strong>. Lưu thay đổi sẽ đưa nó về
          hàng chờ duyệt và <strong>tạm ẩn khỏi cửa hàng</strong> cho tới
          khi được duyệt lại.
        </Alert>
      )}
      <Field label="Tên sản phẩm" htmlFor={`s-ten-${p.id}`}>
        <Input id={`s-ten-${p.id}`} value={f.name} onChange={doi("name")} required />
      </Field>
      <Field label="Mô tả" htmlFor={`s-mota-${p.id}`}>
        <Textarea
          id={`s-mota-${p.id}`}
          value={f.description}
          onChange={doi("description")}
          rows={3}
        />
      </Field>
      <Field label="Thành phần chất liệu" htmlFor={`s-cl-${p.id}`}>
        <Input
          id={`s-cl-${p.id}`}
          value={f.material_composition}
          onChange={doi("material_composition")}
        />
      </Field>
      <Field label="Hướng dẫn bảo quản" htmlFor={`s-bq-${p.id}`}>
        <Input
          id={`s-bq-${p.id}`}
          value={f.care_instructions}
          onChange={doi("care_instructions")}
        />
      </Field>
      <Field label="Xuất xứ" htmlFor={`s-xx-${p.id}`}>
        <Input
          id={`s-xx-${p.id}`}
          value={f.origin_country}
          onChange={doi("origin_country")}
        />
      </Field>
      <Field
        label="Ảnh sản phẩm"
        htmlFor={`s-anh-${p.id}`}
        hint="Mỗi dòng một ảnh; ảnh đầu là ảnh bìa. Danh sách này THAY THẾ danh sách cũ."
      >
        <Textarea
          id={`s-anh-${p.id}`}
          value={f.images}
          onChange={doi("images")}
          rows={3}
        />
      </Field>
      <p className="actions">
        <Button type="submit" disabled={busy}>
          {dangBan ? "Lưu và gửi duyệt lại" : "Lưu"}
        </Button>
        <Button
          type="button"
          variant="secondary"
          disabled={busy}
          onClick={() => setMo(false)}
        >
          Hủy
        </Button>
      </p>
    </form>
  );
}
