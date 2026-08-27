"use client";

import { isApiError } from "@fc/api-client";
import { Alert, Button, Field, Input } from "@fc/ui";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import * as React from "react";

import { useShop } from "@/lib/shop";

/**
 * Trang đăng nhập.
 *
 * # Đăng nhập KHÔNG bắt buộc để mua hàng
 *
 * Trang này tồn tại cho khách muốn giữ giỏ giữa các thiết bị và xem hồ sơ.
 * Không có chỗ nào trên đường mua hàng ép khách vào đây.
 *
 * # Thông báo lỗi CỐ Ý mơ hồ
 *
 * "Email hoặc mật khẩu không đúng" — không nói cái nào sai. Phân biệt được
 * nghĩa là trang này trả lời câu "email này có tài khoản chưa", và khi đó
 * nó thành công cụ dò danh sách email.
 */
/**
 * Bọc Suspense vì `useSearchParams` đọc URL — thứ CHƯA biết lúc dựng
 * trang tĩnh. Không bọc thì Next.js không prerender được trang này.
 */
export default function LoginPage() {
  return (
    <React.Suspense fallback={<p className="muted">Đang tải…</p>}>
      <LoginForm />
    </React.Suspense>
  );
}

function LoginForm() {
  const { login } = useShop();
  const router = useRouter();
  const search = useSearchParams();
  const next = search.get("next") || "/tai-khoan";

  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(email, password);
      router.push(next);
    } catch (err) {
      setError(
        isApiError(err) && err.status === 401
          ? "Email hoặc mật khẩu không đúng"
          : isApiError(err)
            ? err.message
            : "Không kết nối được máy chủ",
      );
      setBusy(false);
    }
  }

  return (
    <div style={{ maxWidth: 420 }}>
      <h1>Đăng nhập</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      <form onSubmit={onSubmit} className="stack">
        <Field label="Email" htmlFor="email">
          <Input
            id="email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </Field>

        <Field label="Mật khẩu" htmlFor="password">
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </Field>

        <div className="actions">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Đang đăng nhập…" : "Đăng nhập"}
          </Button>
        </div>
      </form>

      <p className="muted">
        Chưa có tài khoản? <Link href="/dang-ky">Đăng ký</Link>
      </p>
      <p className="muted">
        Bạn <strong>không cần tài khoản</strong> để mua hàng hay tra cứu đơn —{" "}
        <Link href="/orders">tra đơn bằng mã đơn và số điện thoại</Link>.
      </p>
    </div>
  );
}
