"use client";

import { Button, Field, Input } from "@fc/ui";
import { useRouter } from "next/navigation";
import * as React from "react";

/**
 * Tra cứu đơn hàng.
 *
 * # Vì sao là một FORM chứ không phải danh sách
 *
 * Khách VÃNG LAI không có tài khoản, nên không có "đơn hàng của tôi" để
 * liệt kê — họ chứng minh quyền xem bằng mã đơn + số điện thoại. Đó cũng là
 * cặp thông tin duy nhất họ có: mã đơn nằm trong email xác nhận.
 *
 * Số điện thoại KHÔNG lưu ở đây, chỉ chuyển qua trang chi tiết trong một
 * lần điều hướng.
 */
export default function OrderLookupPage() {
  const router = useRouter();
  const [orderNumber, setOrderNumber] = React.useState("");
  const [phone, setPhone] = React.useState("");

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const key = orderNumber.trim();
    if (!key) return;
    router.push(
      `/orders/${encodeURIComponent(key)}?phone=${encodeURIComponent(phone.trim())}`,
    );
  }

  return (
    <div>
      <h1>Tra cứu đơn hàng</h1>
      <p className="muted">
        Nhập mã đơn trong email xác nhận và số điện thoại bạn đã dùng khi đặt.
      </p>

      <form onSubmit={onSubmit} className="stack" style={{ maxWidth: 420 }}>
        <Field label="Mã đơn hàng" htmlFor="order_number" hint="Ví dụ: FC-2026-08-000001">
          <Input
            id="order_number"
            value={orderNumber}
            onChange={(e) => setOrderNumber(e.target.value)}
            required
          />
        </Field>

        <Field label="Số điện thoại" htmlFor="phone">
          <Input
            id="phone"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            required
          />
        </Field>

        <div className="actions">
          <Button type="submit">Tra cứu</Button>
        </div>
      </form>
    </div>
  );
}
