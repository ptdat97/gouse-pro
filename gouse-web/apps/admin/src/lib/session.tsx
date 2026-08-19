"use client";

import {
  ApiClient,
  getMe,
  isApiError,
  login as apiLogin,
  logout as apiLogout,
  type AdminMe,
} from "@fc/api-client";
import * as React from "react";

/**
 * Phiên đăng nhập của nhân viên.
 *
 * # Access token nằm trong BỘ NHỚ của ApiClient
 *
 * Không localStorage, không sessionStorage, không cookie đọc được bằng JS.
 * Tải lại trang là mất token — và đó là lý do có bước "khôi phục phiên"
 * dưới đây: nó gọi refresh, dùng cookie httpOnly mà trình duyệt tự gửi.
 *
 * # Vai trò ở đây CHỈ để hiển thị
 *
 * Ẩn một mục menu không phải là bảo mật. Backend kiểm tra lại vai trò ở
 * MỌI endpoint — người dùng gọi API trực tiếp được, bỏ qua hoàn toàn giao
 * diện. Xem docs/08-frontend/frontend-architecture.md mục 8.
 */

interface SessionValue {
  api: ApiClient;
  me: AdminMe | null;
  /** Đang khôi phục phiên lúc tải trang. */
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Vai trò — CHỈ dùng để dựng menu, không phải để bảo vệ dữ liệu. */
  hasRole: (...roles: string[]) => boolean;
}

const SessionContext = React.createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [me, setMe] = React.useState<AdminMe | null>(null);
  const [loading, setLoading] = React.useState(true);

  const api = React.useMemo(
    () =>
      new ApiClient({
        baseUrl:
          process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
        onSessionExpired: () => setMe(null),
      }),
    [],
  );

  // Khôi phục phiên khi tải trang.
  //
  // Access token đã mất theo bộ nhớ, nhưng refresh token vẫn ở cookie. Một
  // lời gọi /admin/me sẽ nhận 401, client tự làm mới rồi gửi lại — nên chỗ
  // này không cần biết gì về cơ chế đó.
  React.useEffect(() => {
    let cancelled = false;

    getMe(api)
      .then((v) => {
        if (!cancelled) setMe(v);
      })
      .catch(() => {
        // Chưa đăng nhập là trạng thái BÌNH THƯỜNG lúc mở trang, không
        // phải lỗi cần báo.
        if (!cancelled) setMe(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [api]);

  const value: SessionValue = {
    api,
    me,
    loading,
    login: async (email, password) => {
      await apiLogin(api, email, password);
      setMe(await getMe(api));
    },
    logout: async () => {
      try {
        await apiLogout(api);
      } catch (e) {
        // Đăng xuất là idempotent ở backend. Lỗi mạng không được giữ người
        // dùng ở lại trong phiên — xóa trạng thái cục bộ rồi thôi.
        if (!isApiError(e)) console.warn("đăng xuất:", e);
      }
      setMe(null);
    },
    hasRole: (...roles) => {
      if (!me) return false;
      // ADMIN thấy mọi thứ. Vai trò lạ (server thêm mới) KHÔNG làm crash —
      // chỉ đơn giản là không khớp.
      if (me.roles.includes("ADMIN")) return true;
      return roles.some((r) => me.roles.includes(r as never));
    },
  };

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
}

export function useSession(): SessionValue {
  const ctx = React.useContext(SessionContext);
  if (!ctx) {
    throw new Error("useSession phải nằm trong <SessionProvider>");
  }
  return ctx;
}
