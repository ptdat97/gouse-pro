"use client";

import { isApiError, registerCustomer } from "@fc/api-client";
import { Alert, Button, Field, Input } from "@fc/ui";
import Link from "next/link";
import { useRouter } from "next/navigation";
import * as React from "react";

import { useShop } from "@/lib/shop";

/**
 * Trang đăng ký.
 *
 * # Đăng ký xong TỰ ĐĂNG NHẬP
 *
 * Backend cố ý không trả token ở `register` (phát hành token là việc của
 * module identity). Trang này gọi `login` ngay sau đó — khách không thấy
 * bước thừa nào.
 *
 * # Email đã dùng có HAI lý do, và phải nói rõ lý do nào
 *
 * "đã có tài khoản" → đăng nhập. "đã đặt hàng vãng lai" → tra đơn bằng mã
 * đơn. Trả chung một thông báo đẩy nhóm thứ hai vào đường cụt: họ bấm quên
 * mật khẩu cho một tài khoản không tồn tại.
 *
 * Backend đã phân biệt sẵn trong `message`, nên trang chỉ cần hiển thị
 * nguyên văn thay vì tự đoán.
 */
export default function RegisterPage() {
  const { api, login } = useShop();
  const router = useRouter();

  const [form, setForm] = React.useState({
    email: "",
    password: "",
    name: "",
    phone: "",
  });
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);

  function set(k: keyof typeof form) {
    return (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((f) => ({ ...f, [k]: e.target.value }));
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await registerCustomer(api, {
        email: form.email,
        password: form.password,
        name: form.name || undefined,
        phone: form.phone || undefined,
      });
      await login(form.email, form.password);
      router.push("/tai-khoan");
    } catch (err) {
      setError(
        isApiError(err)
          ? err.message
          : "Không kết nối được máy chủ, vui lòng thử lại",
      );
      setBusy(false);
    }
  }

  return (
    <div style={{ maxWidth: 420 }}>
      <h1>Đăng ký</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      <form onSubmit={onSubmit} className="stack">
        <Field label="Email" htmlFor="email">
          <Input id="email" type="email" autoComplete="email"
            value={form.email} onChange={set("email")} required />
        </Field>

        <Field
          label="Mật khẩu"
          htmlFor="password"
          hint="Tối thiểu 8 ký tự"
        >
          <Input id="password" type="password" autoComplete="new-password"
            minLength={8} value={form.password} onChange={set("password")} required />
        </Field>

        <Field label="Họ tên" htmlFor="name">
          <Input id="name" autoComplete="name"
            value={form.name} onChange={set("name")} />
        </Field>

        <Field label="Số điện thoại" htmlFor="phone">
          <Input id="phone" autoComplete="tel"
            value={form.phone} onChange={set("phone")} />
        </Field>

        <div className="actions">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Đang tạo tài khoản…" : "Đăng ký"}
          </Button>
        </div>
      </form>

      <p className="muted">
        Đã có tài khoản? <Link href="/dang-nhap">Đăng nhập</Link>
      </p>
    </div>
  );
}
