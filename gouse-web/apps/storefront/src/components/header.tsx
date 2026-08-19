"use client";

import Link from "next/link";
import * as React from "react";

import { useShop } from "@/lib/shop";

/**
 * Thanh điều hướng.
 *
 * # Số trên biểu tượng giỏ chỉ hiện KHI ĐÃ TẢI XONG
 *
 * Hiện "0" trong lúc đang tải rồi nhảy lên "3" làm khách tưởng vừa mất
 * hàng. Thà chưa hiện gì.
 */
export function Header() {
  const { itemCount, cartLoading, me, authLoading } = useShop();

  return (
    <header className="header">
      <div className="header__inner">
        <Link href="/" className="header__brand">
          Fashion Commerce
        </Link>

        <nav className="header__nav">
          <Link href="/orders">Tra cứu đơn</Link>

          {/* Chưa khôi phục xong phiên thì KHÔNG hiện gì: nhấp nháy
              "Đăng nhập" rồi đổi thành tên khách trông như lỗi. */}
          {!authLoading &&
            (me ? (
              <Link href="/tai-khoan">{me.name || "Tài khoản"}</Link>
            ) : (
              <Link href="/dang-nhap">Đăng nhập</Link>
            ))}

          <Link href="/cart" className="header__cart">
            Giỏ hàng
            {!cartLoading && itemCount > 0 && (
              <span className="header__badge" aria-label={`${itemCount} món`}>
                {itemCount}
              </span>
            )}
          </Link>
        </nav>
      </div>
    </header>
  );
}
