"use client";

import { Alert, Button } from "@fc/ui";
import Link from "next/link";
import { usePathname } from "next/navigation";
import * as React from "react";

import { useSession } from "@/lib/session";

import { LoginForm } from "./login-form";

/**
 * Vỏ ứng dụng: điều hướng, thông tin người đăng nhập, chặn khi chưa đăng
 * nhập.
 *
 * # Menu lọc theo vai trò CHỈ là trải nghiệm
 *
 * Ẩn mục "Tài chính" với người không có quyền là để họ không bấm vào rồi
 * nhận 403. Nó KHÔNG bảo vệ dữ liệu — backend kiểm tra lại ở mọi endpoint.
 */

interface NavItem {
  href: string;
  label: string;
  /** Vai trò thấy được mục này. ADMIN luôn thấy tất cả. */
  roles: string[];
}

const NAV: NavItem[] = [
  { href: "/sellers", label: "Nhà bán", roles: ["OPS_MERCHANDISING"] },
  { href: "/orders", label: "Đơn hàng", roles: ["OPS_SUPPORT"] },
  { href: "/audit-log", label: "Nhật ký thao tác", roles: [] },

  // CHỈ ADMIN: những tham số này quyết định cách hệ thống chấm điểm nhà
  // bán, nên quyền đổi để ở nhóm nhỏ nhất. Danh sách rỗng nghĩa là chỉ
  // ADMIN thấy — xem chú thích ở đầu tệp.
  { href: "/config", label: "Cấu hình vận hành", roles: [] },
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

  if (!me) {
    return <LoginForm />;
  }

  // Nhật ký thao tác CHỈ vai trò ADMIN — khớp ràng buộc ở backend
  // (admin-api.md mục 7). Hiện mục này cho người khác chỉ dẫn tới 403.
  const visible = NAV.filter((item) =>
    item.roles.length === 0 ? hasRole("ADMIN") : hasRole(...item.roles),
  );

  return (
    <div className="shell">
      <nav className="sidebar" aria-label="Điều hướng chính">
        <div className="sidebar__brand">Quản trị</div>

        <div className="sidebar__nav">
          {visible.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="sidebar__link"
              aria-current={
                pathname.startsWith(item.href) ? "page" : undefined
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
        {/*
          Tài khoản có quyền tài chính vẫn chỉ dùng mật khẩu: 2FA chưa
          triển khai (P3-5). Nêu rõ cho người dùng biết thay vì im lặng —
          họ là người chịu hậu quả nếu mật khẩu bị lộ.
        */}
        {me.requires_two_factor && (
          <Alert tone="warning" title="Tài khoản này chưa bật xác thực hai lớp">
            Bạn có quyền thực hiện thao tác tài chính. Hãy dùng mật khẩu
            mạnh và không đăng nhập trên máy dùng chung.
          </Alert>
        )}

        {children}
      </main>
    </div>
  );
}
