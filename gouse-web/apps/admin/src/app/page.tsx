"use client";

import { useRouter } from "next/navigation";
import * as React from "react";

import { Shell } from "@/components/shell";
import { useSession } from "@/lib/session";

/**
 * Trang gốc: đưa người dùng tới màn hình làm việc ĐẦU TIÊN của họ.
 *
 * Không có dashboard tổng quan — nhân viên vận hành vào đây để làm việc cụ
 * thể, và một trang toàn biểu đồ chỉ thêm một cú bấm trước khi tới việc
 * thật (admin.md mục 8).
 */
export default function HomePage() {
  const { me, hasRole } = useSession();
  const router = useRouter();

  React.useEffect(() => {
    if (!me) return;
    if (hasRole("OPS_MERCHANDISING")) router.replace("/sellers");
    else if (hasRole("OPS_SUPPORT")) router.replace("/orders");
    else if (hasRole("ADMIN")) router.replace("/audit-log");
  }, [me, hasRole, router]);

  return (
    <Shell>
      <p>Đang chuyển tới màn hình làm việc…</p>
    </Shell>
  );
}
