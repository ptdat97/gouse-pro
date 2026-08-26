import type { operations } from "@fc/types/openapi";
import type { ApiClient } from "./client";

/**
 * Các lời gọi API của cửa hàng.
 *
 * # Kiểu lấy TỪ đặc tả OpenAPI, không viết tay
 *
 * Cùng lý do với `admin.ts`: viết tay `interface Product` nghĩa là frontend
 * tin vào một hợp đồng mà backend không cam kết.
 *
 * # Khách VÃNG LAI là mặc định, không phải ngoại lệ
 *
 * Không lời gọi nào ở đường mua hàng cần access token. Danh tính người mua
 * đến từ cookie `shopper_session` mà backend tự cấp — nên `credentials:
 * "include"` trong client là BẮT BUỘC, không phải tùy chọn: thiếu nó thì
 * mỗi request là một khách khác nhau và giỏ hàng không bao giờ giữ được gì.
 */

type Ok<T extends { responses: { 200: { content: { "application/json": unknown } } } }> =
  T["responses"][200]["content"]["application/json"];

type Created<T extends { responses: { 201: { content: { "application/json": unknown } } } }> =
  T["responses"][201]["content"]["application/json"];

// ---------------------------------------------------------------- Catalog

export type ProductList = Ok<operations["listProducts"]>;
export type ProductDetail = Ok<operations["getProduct"]>;
export type ProductOffers = Ok<operations["listProductOffers"]>;
export type SearchResult = Ok<operations["search"]>;

export interface ProductQuery {
  limit?: number;
  cursor?: string;
  category_id?: string;
  brand_id?: string;
  [param: string]: string | number | undefined;
}

export function listProducts(api: ApiClient, q: ProductQuery = {}): Promise<ProductList> {
  return api.get<ProductList>("/api/v1/products", q);
}

/**
 * Tra NHIỀU sản phẩm theo mã, trong MỘT lượt gọi.
 *
 * Dùng khi trang có danh sách mã mà không có tên hay ảnh — rõ nhất là danh
 * sách yêu thích: module `customer` nằm cùng tầng với `product` nên chỉ trả
 * `product_id`.
 *
 * Gọi `getProduct` cho từng mã là vấn đề N+1: danh sách 30 món thành 30
 * lượt đi-về.
 *
 * Trả về theo ĐÚNG THỨ TỰ mã được hỏi. Mã không tồn tại hoặc sản phẩm chưa
 * duyệt thì VẮNG MẶT — nên độ dài kết quả có thể ngắn hơn danh sách hỏi.
 */
export function listProductsByIds(
  api: ApiClient,
  ids: string[],
): Promise<ProductList["data"]> {
  // Danh sách rỗng thì KHÔNG gọi mạng: `ids=` rỗng sẽ bị bỏ qua và server
  // trả về toàn bộ catalog — đúng thứ ngược lại với ý định.
  if (ids.length === 0) return Promise.resolve([]);

  return api
    .get<ProductList>("/api/v1/products", { ids: ids.join(",") })
    .then((res) => res.data ?? []);
}

// Tên khác `SellerList` của admin.ts: hai endpoint khác nhau, hai tập
// trường khác nhau. Trùng tên là mời gọi dùng nhầm.
type SellerLookup = Ok<operations["lookupSellers"]>;
export type SellerRef = NonNullable<SellerLookup["data"]>[number];

/**
 * Tra hồ sơ nhà bán theo LÔ.
 *
 * Endpoint offer cố ý chỉ trả `seller_id` — ghép dữ liệu là việc của
 * TRANG, không phải của ENDPOINT. Nếu mỗi offer kéo theo cả hồ sơ nhà bán
 * thì cùng một nhà bán bị lặp ở mọi offer của họ, và những lời gọi không
 * hiển thị tên ai vẫn phải trả giá.
 *
 * Đổi lại, TRANG phải gọi thêm một lượt — MỘT lượt cho cả danh sách, không
 * phải một lượt mỗi offer.
 *
 * Trả về theo đúng thứ tự hỏi. Mã không tồn tại thì VẮNG MẶT, nên kết quả
 * có thể ngắn hơn danh sách hỏi — bên gọi phải chịu được điều đó.
 */
export function listSellersByIds(
  api: ApiClient,
  ids: string[],
): Promise<SellerRef[]> {
  // Danh sách rỗng thì KHÔNG gọi mạng: `ids=` rỗng bị máy chủ từ chối với
  // 400, và một lỗi đỏ trong console cho một câu hỏi không ai hỏi.
  if (ids.length === 0) return Promise.resolve([]);

  return api
    .get<SellerLookup>("/api/v1/sellers", { ids: ids.join(",") })
    .then((res) => res.data ?? []);
}

type BuyBoxList = Ok<operations["listBuyBoxPrices"]>;
export type BuyBoxPrice = NonNullable<BuyBoxList["data"]>[number];

/**
 * Tra giá buy box của NHIỀU sản phẩm.
 *
 * Danh mục cố ý KHÔNG chứa giá: giá thuộc về offer, và module `product`
 * cùng tầng với `marketplace` nên không gọi được. Trang tự ghép hai nguồn
 * — cùng mẫu với `listSellersByIds`.
 *
 * Giá lấy từ BUY BOX, tức giá khách thật sự mua được: offer hết hàng và
 * nhà bán bị đình chỉ đã bị loại. Sản phẩm không có offer nào bán được thì
 * VẮNG MẶT, nên bên gọi phải chịu được việc thiếu giá.
 */
export function listBuyBoxPrices(
  api: ApiClient,
  productIds: string[],
): Promise<BuyBoxPrice[]> {
  if (productIds.length === 0) return Promise.resolve([]);

  return api
    .get<BuyBoxList>("/api/v1/offers/buy-box", {
      product_ids: productIds.join(","),
    })
    .then((res) => res.data ?? []);
}

export function getProduct(api: ApiClient, productId: string): Promise<ProductDetail> {
  return api.get<ProductDetail>(`/api/v1/products/${productId}`);
}

/**
 * Các lời chào bán của một sản phẩm.
 *
 * Tách khỏi `getProduct` vì đây là hai câu hỏi khác nhau: "sản phẩm này là
 * gì" (ổn định, cache được) và "ai đang bán, giá bao nhiêu, còn hàng
 * không" (đổi liên tục).
 */
export function listProductOffers(
  api: ApiClient,
  productId: string,
): Promise<ProductOffers> {
  return api.get<ProductOffers>(`/api/v1/products/${productId}/offers`);
}

export function search(api: ApiClient, q: string, limit?: number): Promise<SearchResult> {
  return api.get<SearchResult>("/api/v1/search", { q, limit });
}

// ---------------------------------------------------------------- Giỏ hàng

export type Cart = Ok<operations["getCart"]>;

export function getCart(api: ApiClient): Promise<Cart> {
  return api.get<Cart>("/api/v1/cart");
}

/**
 * Thêm món vào giỏ.
 *
 * Nhận `offerId`, KHÔNG phải `skuId`: khách mua lời chào bán của một nhà bán
 * cụ thể, và cùng một SKU có thể có nhiều nhà bán với giá khác nhau.
 */
export function addCartItem(
  api: ApiClient,
  offerId: string,
  quantity: number,
): Promise<Cart> {
  return api.post<Cart>("/api/v1/cart/items", { offer_id: offerId, quantity });
}

export function updateCartItem(
  api: ApiClient,
  cartItemId: string,
  quantity: number,
): Promise<Cart> {
  return api.patch<Cart>(`/api/v1/cart/items/${cartItemId}`, { quantity });
}

export function removeCartItem(api: ApiClient, cartItemId: string): Promise<Cart> {
  return api.del<Cart>(`/api/v1/cart/items/${cartItemId}`);
}

// ---------------------------------------------------------------- Thanh toán

export type Checkout = Ok<operations["getCheckout"]>;
export type CheckoutStarted = Created<operations["startCheckout"]>;
export type OrderPlaced = Created<operations["completeCheckout"]>;

export interface ShippingAddressInput {
  recipient_name: string;
  phone: string;
  street_address: string;
  ward?: string;
  district?: string;
  province: string;
  country_code: string;
}

/**
 * Mở phiên thanh toán.
 *
 * Đây là bước GIỮ TỒN KHO: từ đây khách có 15 phút, và giá được đóng băng.
 * Gọi nó sớm hơn cần thiết là khóa hàng của người khác mà không ai mua.
 */
export function startCheckout(
  api: ApiClient,
  cartId: string,
  guest?: { email?: string; phone?: string },
): Promise<CheckoutStarted> {
  return api.post<CheckoutStarted>("/api/v1/checkout", {
    cart_id: cartId,
    guest_email: guest?.email,
    guest_phone: guest?.phone,
  });
}

export function getCheckout(api: ApiClient, checkoutId: string): Promise<Checkout> {
  return api.get<Checkout>(`/api/v1/checkout/${checkoutId}`);
}

export function setCheckoutShippingAddress(
  api: ApiClient,
  checkoutId: string,
  address: ShippingAddressInput,
): Promise<Checkout> {
  return api.patch<Checkout>(
    `/api/v1/checkout/${checkoutId}/shipping-address`,
    address,
  );
}

/**
 * Chọn phương thức vận chuyển.
 *
 * Chỉ gửi TÊN phương thức — phí do máy chủ tra. Gửi phí lên là để khách tự
 * đặt phí ship 0đ cho mình.
 */
export function setCheckoutShippingMethod(
  api: ApiClient,
  checkoutId: string,
  method: "STANDARD" | "EXPRESS",
): Promise<Checkout> {
  return api.patch<Checkout>(`/api/v1/checkout/${checkoutId}/shipping-method`, {
    shipping_method: method,
  });
}

export function applyCheckoutCoupon(
  api: ApiClient,
  checkoutId: string,
  code: string,
): Promise<Checkout> {
  return api.post<Checkout>(`/api/v1/checkout/${checkoutId}/coupon`, { code });
}

/**
 * Hoàn tất và tạo đơn.
 *
 * # `idempotencyKey` PHẢI gắn với PHIÊN, không phải với lần bấm
 *
 * Khách bấm "Đặt hàng" hai lần, hoặc mạng chậm rồi client tự gửi lại. Để
 * client sinh khóa mới mỗi lần gọi thì hai lần bấm là hai đơn và hai lần
 * trừ tiền. Khóa lấy từ `checkoutId` nên mọi lần thử lại đều trùng khóa.
 */
export function completeCheckout(
  api: ApiClient,
  checkoutId: string,
  paymentMethod: "CARD" | "BANK_TRANSFER" | "E_WALLET" | "COD",
): Promise<OrderPlaced> {
  return api.post<OrderPlaced>(
    `/api/v1/checkout/${checkoutId}/complete`,
    { payment_method: paymentMethod },
    { idempotencyKey: checkoutId.replace(/[^A-Za-z0-9]/g, "") },
  );
}

// ---------------------------------------------------------------- Đơn hàng

export type MyOrders = Ok<operations["listMyOrders"]>;
export type OrderView = Ok<operations["getOrder"]>;
export type OrderShipments = Ok<operations["listOrderShipments"]>;

export function listMyOrders(api: ApiClient, limit?: number): Promise<MyOrders> {
  return api.get<MyOrders>("/api/v1/orders", { limit });
}

/**
 * Xem một đơn của CHÍNH MÌNH.
 *
 * Tên khác `getOrder` của admin có chủ đích: hai hàm gọi hai endpoint khác
 * nhau với hai quy tắc quyền khác hẳn. Trùng tên thì sớm muộn có người
 * dùng nhầm hàm của admin trong trang khách.
 *
 * `key` nhận CẢ mã đơn (`ord_...`) lẫn mã hiển thị (`FC-2026-08-000001`) —
 * khách vãng lai chỉ có mã hiển thị trong email xác nhận.
 *
 * Khách vãng lai PHẢI kèm số điện thoại: đó là thứ duy nhất chứng minh đơn
 * này của họ.
 */
export function getMyOrder(
  api: ApiClient,
  key: string,
  guestPhone?: string,
): Promise<OrderView> {
  return api.get<OrderView>(
    `/api/v1/orders/${encodeURIComponent(key)}`,
    undefined,
    guestPhone ? { "X-Guest-Phone": guestPhone } : undefined,
  );
}

// ---------------------------------------------------------------- Tài khoản

export type RegisterResult = Created<operations["registerCustomer"]>;
export type MyProfile = Ok<operations["getMyProfile"]>;
export type MyAddresses = Ok<operations["listMyAddresses"]>;
export type MyWishlist = Ok<operations["getMyWishlist"]>;
export type CartMerged = Ok<operations["mergeCartOnLogin"]>;

export interface RegisterInput {
  email: string;
  password: string;
  name?: string;
  phone?: string;
}

/**
 * Đăng ký tài khoản khách hàng.
 *
 * KHÔNG trả token — gọi `login` ngay sau đó. Phát hành token là việc của
 * module identity; làm ở hai chỗ nghĩa là nhân bản logic quản lý phiên.
 *
 * Trả `409` khi email đã dùng, với HAI lý do khác nhau mà giao diện phải
 * phân biệt: đã có tài khoản (→ đăng nhập) và đã đặt hàng vãng lai (→ tra
 * đơn bằng mã + số điện thoại).
 */
export function registerCustomer(
  api: ApiClient,
  input: RegisterInput,
): Promise<RegisterResult> {
  return api.post<RegisterResult>("/api/v1/auth/register", input);
}

/**
 * Gộp giỏ vãng lai vào giỏ tài khoản.
 *
 * Gọi NGAY SAU khi đăng nhập. Không gọi thì khách thêm hàng lúc chưa đăng
 * nhập, đăng nhập xong thấy giỏ trống — và họ nghĩ hệ thống mất dữ liệu.
 *
 * `warnings` PHẢI được hiển thị: món không gộp trọn vẹn được nằm ở đó.
 */
export function mergeCartOnLogin(api: ApiClient): Promise<CartMerged> {
  return api.post<CartMerged>("/api/v1/cart/merge");
}

export function getMyProfile(api: ApiClient): Promise<MyProfile> {
  return api.get<MyProfile>("/api/v1/me");
}

export function updateMyProfile(
  api: ApiClient,
  input: { name?: string; phone?: string },
): Promise<MyProfile> {
  return api.patch<MyProfile>("/api/v1/me", input);
}

export function listMyAddresses(api: ApiClient): Promise<MyAddresses> {
  return api.get<MyAddresses>("/api/v1/me/addresses");
}

export function addMyAddress(
  api: ApiClient,
  address: ShippingAddressInput & { is_default?: boolean },
): Promise<unknown> {
  return api.post("/api/v1/me/addresses", address);
}

export function getMyWishlist(api: ApiClient): Promise<MyWishlist> {
  return api.get<MyWishlist>("/api/v1/me/wishlist");
}

export function addWishlistItem(
  api: ApiClient,
  productId: string,
  notifyWhenAvailable = false,
): Promise<unknown> {
  return api.post("/api/v1/me/wishlist", {
    product_id: productId,
    notify_when_available: notifyWhenAvailable,
  });
}

/**
 * Lô giao của một đơn.
 *
 * Tách khỏi `getMyOrder` vì hai module khác nhau sở hữu hai nửa dữ liệu:
 * `order` giữ dòng hàng và tiền, `fulfillment` giữ tiến độ giao. Ghép là
 * việc của TRANG — nó khớp `order_line_ids` với `lines` đã có sẵn, nên
 * không cần thêm lượt gọi nào.
 */
export function listOrderShipments(
  api: ApiClient,
  key: string,
  guestPhone?: string,
): Promise<OrderShipments> {
  return api.get<OrderShipments>(
    `/api/v1/orders/${encodeURIComponent(key)}/shipments`,
    undefined,
    guestPhone ? { "X-Guest-Phone": guestPhone } : undefined,
  );
}
