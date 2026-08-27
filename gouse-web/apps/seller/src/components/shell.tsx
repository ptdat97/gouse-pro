"use client";

import { Alert, Button } from "@fc/ui";
import Link from "next/link";
import { usePathname } from "next/navigation";
import * as React from "react";

import { useSession } from "@/lib/session";

import { LoginForm } from "./login-form";

/**
 * Vỏ ứng dụng của NHÀ BÁN.
 *
 * # Vai trò ở đây CHỈ để dựng menu
 *
 * Backend kiểm tra lại ở mọi endpoint, và ranh giới giữa các nhà bán nằm
 * trong TRUY VẤN — không phải ở việc ẩn một mục menu.
 *
 * # Tài khoản không gắn nhà bán nào vẫn đăng nhập được
 *
 * Backend trả `403` ở mọi endpoint của nhà bán. Nói rõ điều đó thay vì để
 * họ thấy màn hình trống và tưởng hệ thống hỏng.
 */

const NAV = [
  { href: "/", label: "Việc cần làm" },
  { href: "/offers", label: "Hàng đang bán" },
];

export function Shell({ children }: { children: React.ReactNode }) {
  const { me, loading, logout, hasRole } = useSession();
  const pathname = usePathname();

  if (loading) {
    return (
      <div className="login">
        <p>Đang khôi phục phiên…</p>
      </div>
    );
  }

  if (!me) return <LoginForm />;

  const isSeller = hasRole("SELLER_OWNER", "SELLER_STAFF");

  return (
    <div className="shell">
      <nav className="sidebar" aria-label="Điều hướng chính">
        <div className="sidebar__brand">Nhà bán</div>

        <div className="sidebar__nav">
          {NAV.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="sidebar__link"
              aria-current={
                item.href === "/"
                  ? pathname === "/"
                    ? "page"
                    : undefined
                  : pathname.startsWith(item.href)
                    ? "page"
                    : undefined
              }
            >
              {item.label}
            </Link>
          ))}
        </div>

        <div className="sidebar__user">
          <div>{me.display_name || me.email}</div>
          <div>{me.roles.join(" · ")}</div>
          <div style={{ marginTop: "var(--space-3)" }}>
            <Button onClick={() => void logout()}>Đăng xuất</Button>
          </div>
        </div>
      </nav>

      <main className="main">
        {!isSeller && (
          <Alert tone="warning" title="Tài khoản này chưa gắn với nhà bán nào">
            Bạn đăng nhập được nhưng chưa xem được đơn hàng hay sản phẩm.
            Liên hệ nền tảng để gắn tài khoản với gian hàng của bạn.
          </Alert>
        )}

        {children}
      </main>
    </div>
  );
}
