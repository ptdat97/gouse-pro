"use client";

import {
  addMyAddress,
  getMyWishlist,
  isApiError,
  listMyAddresses,
  updateMyProfile,
  type MyAddresses,
  type MyWishlist,
} from "@fc/api-client";
import { Alert, Button, Field, Input } from "@fc/ui";
import Link from "next/link";
import { useRouter } from "next/navigation";
import * as React from "react";

import { dateTime } from "@/lib/format";
import { useShop } from "@/lib/shop";

/**
 * Trang tài khoản: hồ sơ, sổ địa chỉ, danh sách yêu thích.
 *
 * # Ba nhóm dữ liệu trên MỘT trang
 *
 * Chúng đều nhỏ và đều thuộc "thông tin của tôi". Tách ba trang bắt khách
 * điều hướng qua lại để làm những việc thường đi cùng nhau (đổi số điện
 * thoại rồi thêm địa chỉ giao hàng).
 *
 * # Yêu thích chỉ có MÃ sản phẩm
 *
 * Backend trả `product_id` chứ không trả tên và ảnh: module `customer` nằm
 * cùng tầng với `product` nên không gọi được. Ghép là việc của TRANG — và
 * việc ghép đó chưa làm, nên tạm hiển thị mã. Xem README.
 */
export default function AccountPage() {
  const { me, authLoading, logout } = useShop();
  const router = useRouter();

  React.useEffect(() => {
    if (!authLoading && !me) router.replace("/dang-nhap?next=/tai-khoan");
  }, [authLoading, me, router]);

  if (authLoading) return <p className="muted">Đang tải…</p>;
  if (!me) return null;

  return (
    <div>
      <h1>Tài khoản</h1>
      <p className="muted">
        {me.email} · hạng {me.tier}
      </p>

      <ProfileSection />
      <AddressSection />
      <WishlistSection />

      <div className="actions">
        <Button onClick={() => void logout().then(() => router.push("/"))}>
          Đăng xuất
        </Button>
      </div>
    </div>
  );
}

function ProfileSection() {
  const { api, me } = useShop();
  const [name, setName] = React.useState(me?.name ?? "");
  const [phone, setPhone] = React.useState(me?.phone ?? "");
  const [saved, setSaved] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);
    try {
      await updateMyProfile(api, { name, phone });
      setSaved(true);
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không lưu được hồ sơ");
    }
  }

  return (
    <section className="panel">
      <h2>Hồ sơ</h2>
      {error && <Alert tone="danger">{error}</Alert>}
      {saved && <Alert tone="success">Đã lưu.</Alert>}

      <form onSubmit={onSubmit} className="stack">
        <div className="form-row form-row--2">
          <Field label="Họ tên" htmlFor="name">
            <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label="Số điện thoại" htmlFor="phone">
            <Input id="phone" value={phone} onChange={(e) => setPhone(e.target.value)} />
          </Field>
        </div>

        {/* Email KHÔNG sửa được ở đây: đổi email là đổi DANH TÍNH, cần xác
            minh quyền sở hữu địa chỉ mới. Backend từ chối trường này. */}
        <p className="muted">
          Email không đổi được tại đây. Liên hệ hỗ trợ nếu bạn cần thay đổi.
        </p>

        <div className="actions">
          <Button type="submit">Lưu</Button>
        </div>
      </form>
    </section>
  );
}

function AddressSection() {
  const { api } = useShop();
  const [list, setList] = React.useState<MyAddresses | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [adding, setAdding] = React.useState(false);

  const load = React.useCallback(async () => {
    try {
      setList(await listMyAddresses(api));
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không tải được sổ địa chỉ");
    }
  }, [api]);

  React.useEffect(() => {
    void load();
  }, [load]);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setError(null);
    try {
      await addMyAddress(api, {
        recipient_name: String(f.get("recipient_name") ?? ""),
        phone: String(f.get("phone") ?? ""),
        street_address: String(f.get("street_address") ?? ""),
        ward: String(f.get("ward") ?? ""),
        district: String(f.get("district") ?? ""),
        province: String(f.get("province") ?? ""),
        country_code: "VN",
        is_default: (list?.data?.length ?? 0) === 0,
      });
      setAdding(false);
      await load();
    } catch (err) {
      setError(isApiError(err) ? err.message : "Không thêm được địa chỉ");
    }
  }

  const addresses = list?.data ?? [];

  return (
    <section className="panel">
      <h2>Sổ địa chỉ</h2>
      {error && <Alert tone="danger">{error}</Alert>}

      {addresses.length === 0 ? (
        <p className="muted">Bạn chưa lưu địa chỉ nào.</p>
      ) : (
        <ul className="lines">
          {addresses.map((a, i) => (
            <li key={a.id ?? i} className="line">
              <div>
                <strong>{a.recipient_name}</strong>
                {a.is_default && " · mặc định"}
                <div className="muted">{a.phone}</div>
                <div className="muted">
                  {[a.street_address, a.ward, a.district, a.province]
                    .filter(Boolean)
                    .join(", ")}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}

      {adding ? (
        <form onSubmit={onSubmit} className="stack">
          <div className="form-row form-row--2">
            <Field label="Người nhận" htmlFor="a_name">
              <Input id="a_name" name="recipient_name" required />
            </Field>
            <Field label="Số điện thoại" htmlFor="a_phone">
              <Input id="a_phone" name="phone" required />
            </Field>
          </div>
          <Field label="Địa chỉ" htmlFor="a_street">
            <Input id="a_street" name="street_address" required />
          </Field>
          <div className="form-row form-row--2">
            <Field label="Phường/Xã" htmlFor="a_ward">
              <Input id="a_ward" name="ward" />
            </Field>
            <Field label="Quận/Huyện" htmlFor="a_district">
              <Input id="a_district" name="district" />
            </Field>
          </div>
          <Field label="Tỉnh/Thành phố" htmlFor="a_province">
            <Input id="a_province" name="province" required />
          </Field>
          <div className="actions">
            <Button type="submit" variant="primary">Lưu địa chỉ</Button>
            <Button type="button" onClick={() => setAdding(false)}>Hủy</Button>
          </div>
        </form>
      ) : (
        <div className="actions">
          <Button onClick={() => setAdding(true)}>Thêm địa chỉ</Button>
        </div>
      )}
    </section>
  );
}

function WishlistSection() {
  const { api } = useShop();
  const [list, setList] = React.useState<MyWishlist | null>(null);

  React.useEffect(() => {
    void (async () => {
      try {
        setList(await getMyWishlist(api));
      } catch {
        setList(null);
      }
    })();
  }, [api]);

  const items = list?.data ?? [];

  return (
    <section className="panel">
      <h2>Yêu thích</h2>
      {items.length === 0 ? (
        <p className="muted">Bạn chưa lưu sản phẩm nào.</p>
      ) : (
        <ul className="lines">
          {items.map((it, i) => (
            <li key={it.product_id ?? i} className="line">
              <div>
                <Link href={`/products/${it.product_id}`}>{it.product_id}</Link>
                <div className="muted">Lưu lúc {dateTime(it.added_at)}</div>
              </div>
              <div className="muted">
                {it.notify_when_available ? "Báo khi có hàng" : ""}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
