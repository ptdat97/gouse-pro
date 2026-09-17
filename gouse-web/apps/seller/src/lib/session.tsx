"use client";

import {
  ApiClient,
  getMe,
  isApiError,
  login as apiLogin,
  logout as apiLogout,
  type AdminMe,
  type VaiTro,
} from "@fc/api-client";
import * as React from "react";

/**
 * Phiên đăng nhập của NHÀ BÁN.
 *
 * # Dùng chung endpoint `/api/v1/admin/me`
 *
 * Tên đường dẫn nói "admin" nhưng nó KHÔNG chặn theo vai trò — nó trả về
 * "người đang đăng nhập là ai" cho bất kỳ ai đã xác thực. Nhà bán dùng
 * đúng endpoint đó để khôi phục phiên.
 *
 * Đặt lại tên cho đúng nghĩa là việc nên làm, nhưng đổi đường dẫn công khai
 * thì phải sửa cả Admin UI — ghi vào backlog thay vì nhân bản một endpoint
 * thứ hai làm cùng một việc.
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
  /**
   * Vai trò — CHỈ dùng để dựng menu, không phải để bảo vệ dữ liệu.
   *
   * Tham số nhận đúng KIỂU vai trò của hợp đồng, không phải `string`. Gõ
   * sai một vai trò nay là lỗi biên dịch; trước đây nó âm thầm trả `false`
   * và mục menu biến mất mà không ai biết vì sao.
   */
  hasRole: (...roles: VaiTro[]) => boolean;
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
      // KHÔNG còn `as never` ở đây.
      //
      // Phép ép ấy tồn tại vì enum `roles` trong đặc tả thiếu bốn vai trò
      // phổ biến nhất, gồm cả `SELLER_OWNER` — nên trình biên dịch nói
      // đúng rằng giá trị đó "không thể có", và cách đi qua là tắt nó đi.
      // Kiểu bị vô hiệu ở đúng chỗ cần nhất: một phép kiểm phân quyền.
      // Đặc tả sửa 17/09/2026, nên phép ép hết lý do tồn tại.
      return roles.some((r) => me.roles.includes(r));
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
