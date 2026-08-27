"use client";

import { isApiError } from "@fc/api-client";
import { Alert, Button, Field, Input } from "@fc/ui";
import * as React from "react";

import { useSession } from "@/lib/session";

/**
 * Form đăng nhập.
 *
 * # Không đoán giúp người dùng vì sao thất bại
 *
 * Backend trả CÙNG MỘT lỗi cho email không tồn tại, sai mật khẩu, và tài
 * khoản bị treo — có chủ ý, để đường đăng nhập không thành công cụ dò danh
 * sách email. Giao diện hiển thị nguyên thông báo đó, không thêm phỏng
 * đoán kiểu "có thể email chưa đăng ký".
 */
export function LoginForm() {
  const { login } = useSession();
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [submitting, setSubmitting] = React.useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await login(email, password);
    } catch (err) {
      setError(
        isApiError(err)
          ? err.message
          : "Không kết nối được máy chủ. Kiểm tra backend có đang chạy không.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="login">
      <form className="login__card" onSubmit={onSubmit}>
        <h1>Đăng nhập</h1>
        <p className="page__lead" style={{ marginBottom: "var(--space-6)" }}>
          Giao diện vận hành nội bộ.
        </p>

        {error && <Alert tone="danger">{error}</Alert>}

        <Field label="Email" htmlFor="email">
          <Input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            required
            autoFocus
          />
        </Field>

        <Field label="Mật khẩu" htmlFor="password">
          <Input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>

        <Button type="submit" variant="primary" loading={submitting}>
          Đăng nhập
        </Button>
      </form>
    </div>
  );
}
