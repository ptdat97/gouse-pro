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

// --------------------------------------------------------------- Trả hàng

export type MyReturns = Ok<operations["listMyReturns"]>;
export type ReturnUpdated = Ok<operations["approveReturn"]>;

/**
 * Yêu cầu trả hàng của gian hàng tôi.
 *
 * `status` bỏ trống thì trả tất cả. Backend chỉ trả yêu cầu thuộc gian
 * hàng của người gọi — `seller_id` lấy từ token, không từ tham số.
 */
export function listMyReturns(api: ApiClient, status?: string): Promise<MyReturns> {
  return api.get<MyReturns>("/api/v1/seller/returns", { status });
}

/**
 * Duyệt: đồng ý cho khách gửi hàng về. CHƯA hoàn tiền.
 *
 * Tiền chỉ đi ở `receiveReturn`. Hoàn tiền ngay lúc duyệt nghĩa là trả
 * tiền cho một món hàng có thể không bao giờ được gửi đi.
 */
export function approveReturn(api: ApiClient, id: string): Promise<ReturnUpdated> {
  return api.post<ReturnUpdated>(
    `/api/v1/seller/returns/${encodeURIComponent(id)}/approve`,
  );
}

/**
 * Từ chối, BẮT BUỘC kèm lý do.
 *
 * Lý do đi thẳng tới khách. Từ chối không kèm lý do là cách chắc chắn
 * nhất để một yêu cầu trả hàng thành một khiếu nại.
 */
export function rejectReturn(
  api: ApiClient,
  id: string,
  reason: string,
): Promise<ReturnUpdated> {
  return api.post<ReturnUpdated>(
    `/api/v1/seller/returns/${encodeURIComponent(id)}/reject`,
    { reason },
  );
}

/**
 * Xác nhận hàng đã về kho — bước ĐI TIỀN.
 *
 * Bút toán hoàn tiền ghi ở đây. Hàng về nằm ở trạng thái `Returned`, chưa
 * bán lại được; quyết định đó thuộc bước kiểm định.
 */
export function receiveReturn(api: ApiClient, id: string): Promise<ReturnUpdated> {
  return api.post<ReturnUpdated>(
    `/api/v1/seller/returns/${encodeURIComponent(id)}/receive`,
  );
}

export interface InspectLine {
  order_line_id: string;
  passed: boolean;
  note?: string;
}

/**
 * Kiểm định hàng hoàn — mắt xích CUỐI.
 *
 * `passed = true` đưa hàng về `Available`, `false` đưa vào `Damaged`.
 * Không có bước này thì mọi món hàng hoàn nằm chết vĩnh viễn: nhà bán mất
 * cả hàng lẫn tiền.
 *
 * Kiểm theo TỪNG DÒNG: một yêu cầu trả hai món hoàn toàn có thể một món
 * bán lại được và một món hỏng.
 */
export function inspectReturn(
  api: ApiClient,
  id: string,
  lines: InspectLine[],
): Promise<ReturnUpdated> {
  return api.post<ReturnUpdated>(
    `/api/v1/seller/returns/${encodeURIComponent(id)}/inspect`,
    { lines },
  );
}

// ------------------------------------------------------------ Tiền của tôi

export type MyBalance = Ok<operations["getMyBalance"]>;
export type MySettlements = Ok<operations["listMySettlements"]>;
export type MySettlement = Ok<operations["getMySettlement"]>;

/**
 * Số dư theo NĂM trạng thái.
 *
 * Ba trong năm trạng thái hôm nay luôn bằng 0 — `processing`, `on_hold`,
 * `reserve_held` chưa được mô hình hóa ở backend. Đó là con số ĐÚNG, không
 * phải dữ liệu thiếu: chưa có luồng chi trả thì không đồng nào "đang xử
 * lý", chưa có cơ chế giữ tiền thì không đồng nào bị giữ.
 *
 * Giao diện phải nói rõ điều đó thay vì hiện bốn ô 0 đ trông như hỏng.
 */
export function getMyBalance(api: ApiClient): Promise<MyBalance> {
  return api.get<MyBalance>("/api/v1/seller/balance");
}

/** Các đợt đối soát của gian hàng, mới nhất trước. */
export function listMySettlements(api: ApiClient): Promise<MySettlements> {
  return api.get<MySettlements>("/api/v1/seller/settlements");
}

/**
 * Chi tiết một đợt.
 *
 * Hôm nay nó trả đúng những gì danh sách đã trả — chưa có phần tách theo
 * loại khoản (hoa hồng, phí, hoàn hàng). Xem P3-60.
 */
export function getMySettlement(
  api: ApiClient,
  id: string,
): Promise<MySettlement> {
  return api.get<MySettlement>(
    `/api/v1/seller/settlements/${encodeURIComponent(id)}`,
  );
}

// -------------------------------------------------------------- Hiệu suất

export type MyPerformance = Ok<operations["getMyPerformance"]>;

/** Ba kỳ hợp lệ — backend từ chối mọi giá trị khác. */
export type KyHieuSuat = "LAST_7_DAYS" | "LAST_30_DAYS" | "LAST_90_DAYS";

/**
 * Chỉ số hiệu suất gian hàng.
 *
 * Endpoint này trả CẢ những chỉ số nó KHÔNG chấm được, kèm lý do
 * (`not_measured`). Giao diện phải hiện chúng: giấu đi là dựng lại đúng
 * mô hình chấm điểm hộp đen mà đặc tả nói là "tạo tranh chấp không giải
 * quyết được và cảm giác bất công".
 */
export function getMyPerformance(
  api: ApiClient,
  ky?: KyHieuSuat,
): Promise<MyPerformance> {
  return api.get<MyPerformance>(
    "/api/v1/seller/performance",
    ky ? { period: ky } : undefined,
  );
}
