"use client";

import {
  addCartItem as apiAddCartItem,
  ApiClient,
  getCart as apiGetCart,
  getMyProfile,
  isApiError,
  login as apiLogin,
  logout as apiLogout,
  mergeCartOnLogin,
  removeCartItem as apiRemoveCartItem,
  updateCartItem as apiUpdateCartItem,
  type Cart,
  type MyProfile,
} from "@fc/api-client";
import * as React from "react";

import { ThuTuLuot } from "./thu-tu-luot";

/**
 * Trạng thái cửa hàng dùng chung: client gọi API và giỏ hàng.
 *
 * # Đăng nhập là TÙY CHỌN, không phải điều kiện
 *
 * Khách VÃNG LAI phải mua được (mvp.md mục 4). Danh tính người mua đến từ
 * cookie `shopper_session` do backend tự cấp ở request đầu tiên; trình
 * duyệt gửi lại nó nhờ `credentials: "include"`.
 *
 * Đăng nhập chỉ THÊM: giỏ theo người thay vì theo trình duyệt, và mở ra
 * trang hồ sơ, sổ địa chỉ, yêu thích.
 *
 * # GỘP GIỎ ngay sau khi đăng nhập là BẮT BUỘC
 *
 * Không gộp thì khách thêm hàng lúc chưa đăng nhập, đăng nhập xong thấy giỏ
 * trống — và họ nghĩ hệ thống mất dữ liệu của mình. Xem `login` dưới đây.
 *
 * # Giỏ hàng là MỘT nguồn sự thật, nằm ở server
 *
 * Mọi thao tác đều trả về giỏ ĐẦY ĐỦ sau khi sửa, và ta thay thế nguyên
 * trạng thái. KHÔNG sửa cục bộ rồi đồng bộ sau: giá và tình trạng hàng đổi
 * ở server (seller giảm giá, hàng hết), nên bản cục bộ sẽ sai mà không ai
 * biết cho tới bước thanh toán.
 */

interface ShopValue {
  api: ApiClient;

  /** null nghĩa là CHƯA đăng nhập, hoặc chưa khôi phục xong phiên. */
  me: MyProfile["customer"] | null;
  authLoading: boolean;

  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;

  /**
   * Món không gộp trọn vẹn được khi đăng nhập.
   *
   * PHẢI hiển thị cho khách — im lặng bỏ qua nghĩa là họ thấy giỏ ít hàng
   * hơn lúc chưa đăng nhập mà không hiểu vì sao.
   */
  mergeWarnings: string[];

  /** null nghĩa là CHƯA tải xong, khác hẳn giỏ rỗng. */
  cart: Cart | null;
  cartLoading: boolean;
  cartError: string | null;

  refreshCart: () => Promise<void>;
  addItem: (offerId: string, quantity: number) => Promise<void>;
  updateItem: (cartItemId: string, quantity: number) => Promise<void>;
  removeItem: (cartItemId: string) => Promise<void>;

  /** Tổng số món để hiện lên biểu tượng giỏ ở đầu trang. */
  itemCount: number;
}

const ShopContext = React.createContext<ShopValue | null>(null);

export function ShopProvider({ children }: { children: React.ReactNode }) {
  const [cart, setCart] = React.useState<Cart | null>(null);
  const [cartLoading, setCartLoading] = React.useState(true);
  const [cartError, setCartError] = React.useState<string | null>(null);
  const [me, setMe] = React.useState<MyProfile["customer"] | null>(null);
  const [authLoading, setAuthLoading] = React.useState(true);
  const [mergeWarnings, setMergeWarnings] = React.useState<string[]>([]);

  const api = React.useMemo(
    () =>
      new ApiClient({
        baseUrl: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080",
        locale: "vi-VN",
      }),
    [],
  );

  // Chỉ nhận kết quả của lượt MỚI NHẤT.
  //
  // Không có nó, một kịch bản rất thường: trang vừa tải nên `getCart` đang
  // chạy chậm; khách bấm "Thêm vào giỏ"; lời gọi thêm xong TRƯỚC; rồi
  // `getCart` cũ về sau và ghi đè bằng giỏ CHƯA có món vừa thêm.
  //
  // Quy tắc nằm ở `ThuTuLuot` — logic thuần, kiểm được không cần dựng cây
  // React.
  const thuTu = React.useRef(new ThuTuLuot());

  const run = React.useCallback(
    async (fn: () => Promise<Cart>) => {
      const conMoiNhat = thuTu.current.batDau();
      setCartError(null);
      try {
        const moi = await fn();
        if (!conMoiNhat()) return;
        setCart(moi);
      } catch (e) {
        if (!conMoiNhat()) return;
        // Lỗi giỏ hàng KHÔNG được làm trắng trang: khách vẫn duyệt hàng
        // được, và giỏ cũ trên màn hình vẫn đúng cho tới lần sửa sau.
        setCartError(
          isApiError(e) ? e.message : "Không kết nối được máy chủ",
        );
      } finally {
        // Cờ "đang tải" thì luôn tắt, kể cả với lượt đã cũ: nó nói về
        // việc CÓ ĐANG CHỜ hay không.
        setCartLoading(false);
      }
    },
    [],
  );

  const refreshCart = React.useCallback(
    () => run(() => apiGetCart(api)),
    [api, run],
  );

  React.useEffect(() => {
    void refreshCart();
  }, [refreshCart]);

  // Khôi phục phiên lúc tải trang.
  //
  // Access token nằm trong BỘ NHỚ nên tải lại trang là mất. Lời gọi này
  // dùng cookie refresh (HttpOnly) mà trình duyệt tự gửi — thất bại nghĩa
  // là khách chưa đăng nhập, và đó KHÔNG phải lỗi cần báo.
  React.useEffect(() => {
    void (async () => {
      try {
        const p = await getMyProfile(api);
        setMe(p.customer ?? null);
      } catch {
        setMe(null);
      } finally {
        setAuthLoading(false);
      }
    })();
  }, [api]);

  const login = React.useCallback(
    async (email: string, password: string) => {
      await apiLogin(api, email, password);

      // GỘP GIỎ ngay, TRƯỚC khi đọc hồ sơ: thứ tự này quyết định khách có
      // thấy hàng mình vừa thêm hay không.
      try {
        const merged = await mergeCartOnLogin(api);
        setCart({ cart: merged.cart } as Cart);
        setMergeWarnings(
          (merged.warnings ?? []).map(
            (w) =>
              `${w.product_name || "Một món"}: chỉ giữ được ${w.actual_quantity}/${w.wanted_quantity}`,
          ),
        );
      } catch {
        // Gộp hỏng KHÔNG được chặn việc đăng nhập — khách đã xác thực
        // thành công. Đọc lại giỏ để màn hình vẫn đúng.
        await refreshCart();
      }

      const p = await getMyProfile(api);
      setMe(p.customer ?? null);
    },
    [api, refreshCart],
  );

  const logout = React.useCallback(async () => {
    try {
      await apiLogout(api);
    } finally {
      setMe(null);
      setMergeWarnings([]);
      // Giỏ đổi chủ: sau khi đăng xuất, cookie phiên vãng lai cho một giỏ
      // KHÁC. Không đọc lại thì màn hình còn hiện giỏ của tài khoản vừa
      // thoát — đúng thứ không được để lộ trên máy dùng chung.
      await refreshCart();
    }
  }, [api, refreshCart]);

  const value: ShopValue = {
    api,
    me,
    authLoading,
    login,
    logout,
    mergeWarnings,
    cart,
    cartLoading,
    cartError,
    refreshCart,
    addItem: (offerId, quantity) =>
      run(() => apiAddCartItem(api, offerId, quantity)),
    updateItem: (cartItemId, quantity) =>
      run(() => apiUpdateCartItem(api, cartItemId, quantity)),
    removeItem: (cartItemId) => run(() => apiRemoveCartItem(api, cartItemId)),
    itemCount: countItems(cart),
  };

  return <ShopContext.Provider value={value}>{children}</ShopContext.Provider>;
}

export function useShop(): ShopValue {
  const v = React.useContext(ShopContext);
  if (!v) throw new Error("useShop phải nằm trong ShopProvider");
  return v;
}

/**
 * Đếm số món trong giỏ.
 *
 * Đếm DÒNG chứ không cộng số lượng: "3" trên biểu tượng giỏ nghĩa là ba
 * món khác nhau, hợp trực giác hơn "3" khi khách mua ba cái cùng một áo.
 */
export function countItems(cart: Cart | null): number {
  if (!cart?.cart?.groups) return 0;
  return cart.cart.groups.reduce(
    (n, g) => n + (g.items?.length ?? 0),
    0,
  );
}
