import type { operations } from "@fc/types/openapi";
import type { ApiClient } from "./client";

/**
 * Các lời gọi API của NHÀ BÁN.
 *
 * # Định danh nhà bán KHÔNG nằm trong tham số
 *
 * Backend lấy nó từ `AuthContext.SellerIDs` trong token. Không có hàm nào ở
 * đây nhận `seller_id` — cho client truyền vào nghĩa là bất kỳ ai cũng đọc
 * được dữ liệu nhà bán khác chỉ bằng cách đổi một con số.
 */

type Ok<T extends { responses: { 200: { content: { "application/json": unknown } } } }> =
  T["responses"][200]["content"]["application/json"];

type Created<T extends { responses: { 201: { content: { "application/json": unknown } } } }> =
  T["responses"][201]["content"]["application/json"];

// ---------------------------------------------------------------- Offer

export type MyOffers = Ok<operations["listMyOffers"]>;
export type OfferCreated = Created<operations["createOffer"]>;
export type OfferUpdated = Ok<operations["updateOffer"]>;

export interface CreateOfferInput {
  sku_id: string;
  price: { amount: number; currency: string };
  compare_at_price?: { amount: number; currency: string };
  handling_time_hours?: number;
  min_order_quantity?: number;
  max_order_quantity?: number;

  /**
   * Nhập kho ngay khi tạo.
   *
   * Không có nó thì offer HẾT HÀNG từ giây đầu tiên và không có đường nào
   * để nhập: `updateInventory` chỉ SỬA bản ghi đã có.
   */
  initial_inventory?: { stock_location_id?: string; quantity: number };
}

export function listMyOffers(api: ApiClient, status?: string): Promise<MyOffers> {
  return api.get<MyOffers>("/api/v1/seller/offers", { status });
}

export function createOffer(
  api: ApiClient,
  input: CreateOfferInput,
): Promise<OfferCreated> {
  return api.post<OfferCreated>("/api/v1/seller/offers", input);
}

export function updateOffer(
  api: ApiClient,
  offerId: string,
  patch: {
    price?: { amount: number; currency: string };
    handling_time_hours?: number;
    status?: "ACTIVE" | "ARCHIVED";
  },
): Promise<OfferUpdated> {
  return api.patch<OfferUpdated>(`/api/v1/seller/offers/${offerId}`, patch);
}

// ---------------------------------------------------------------- Tồn kho

export type InventoryUpdated = Ok<operations["updateInventory"]>;

/**
 * Kiểm kê: đặt số lượng khả dụng về con số ĐÃ ĐẾM.
 *
 * `quantity` là con số TUYỆT ĐỐI, không phải chênh lệch — đó là cách người
 * kiểm kê nghĩ ("đếm được 40 cái").
 *
 * `reason` BẮT BUỘC ít nhất 5 ký tự: tồn kho lệch mà không có lý do thì
 * không ai đối soát được, và mất mát trông giống hệt sai sót nhập liệu.
 */
export function updateInventory(
  api: ApiClient,
  skuId: string,
  quantity: number,
  reason: string,
): Promise<InventoryUpdated> {
  return api.put<InventoryUpdated>(`/api/v1/seller/inventory/${skuId}`, {
    quantity_available: quantity,
    reason,
  });
}

// ------------------------------------------------------- Đơn thực hiện

export type MyFulfillmentOrders = Ok<operations["listMyFulfillmentOrders"]>;
export type MyFulfillmentOrder = Ok<operations["getMyFulfillmentOrder"]>;
export type ShipResult = Ok<operations["shipFulfillmentOrder"]>;

export function listMyFulfillmentOrders(
  api: ApiClient,
  status?: string,
): Promise<MyFulfillmentOrders> {
  return api.get<MyFulfillmentOrders>("/api/v1/seller/fulfillment-orders", {
    status,
  });
}

export function getMyFulfillmentOrder(
  api: ApiClient,
  id: string,
): Promise<MyFulfillmentOrder> {
  return api.get<MyFulfillmentOrder>(`/api/v1/seller/fulfillment-orders/${id}`);
}

/**
 * Bàn giao cho đơn vị vận chuyển.
 *
 * MÃ VẬN ĐƠN BẮT BUỘC: từ đây hàng ra khỏi tầm kiểm soát của nhà bán, và
 * không có mã thì không ai — kể cả bộ phận hỗ trợ — trả lời được "hàng của
 * tôi đang ở đâu".
 *
 * Backend đi qua mọi bước trung gian còn thiếu (xác nhận → đóng gói → bàn
 * giao), nên nhà bán chỉ cần MỘT thao tác.
 */
export function shipFulfillmentOrder(
  api: ApiClient,
  id: string,
  trackingNumber: string,
  shippingProvider: string,
): Promise<ShipResult> {
  return api.post<ShipResult>(
    `/api/v1/seller/fulfillment-orders/${id}/ship`,
    { tracking_number: trackingNumber, shipping_provider: shippingProvider },
    // Khóa gắn với ĐƠN, không phải lần bấm: bấm hai lần hoặc client tự gửi
    // lại không được ghi nhận hai lần bàn giao.
    { idempotencyKey: id.replace(/[^A-Za-z0-9]/g, "") },
  );
}
