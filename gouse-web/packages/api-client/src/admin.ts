import type { operations } from "@fc/types/openapi";
import type { ApiClient } from "./client";

/**
 * Các lời gọi API quản trị.
 *
 * # Kiểu lấy TỪ đặc tả OpenAPI, không viết tay
 *
 * `operations[...]` là kiểu sinh từ `api/openapi.yaml`. Backend đổi hợp
 * đồng mà quên sinh lại kiểu → `npm run types:check` trong CI đỏ. Backend
 * đổi hợp đồng VÀ sinh lại kiểu → chỗ nào dùng sai sẽ gãy lúc biên dịch,
 * không phải lúc chạy.
 *
 * Đó là toàn bộ lý do tồn tại của tầng này. Viết tay `interface Seller`
 * thì frontend tin vào một hợp đồng mà backend không cam kết.
 */

/** Rút kiểu response 200 của một operation. */
type Ok<T extends { responses: { 200: { content: { "application/json": unknown } } } }> =
  T["responses"][200]["content"]["application/json"];

// ---------------------------------------------------------------- Xác thực

export type LoginResult = Ok<operations["login"]>;
export type AdminMe = Ok<operations["getAdminMe"]>;

export async function login(
  api: ApiClient,
  email: string,
  password: string,
): Promise<LoginResult> {
  const res = await api.post<LoginResult>("/api/v1/auth/login", {
    email,
    password,
  });
  api.setAccessToken(res.access_token);
  return res;
}

export async function logout(api: ApiClient): Promise<void> {
  await api.post<void>("/api/v1/auth/logout");
  api.setAccessToken(null);
}

export function getMe(api: ApiClient): Promise<AdminMe> {
  return api.get<AdminMe>("/api/v1/admin/me");
}

// ---------------------------------------------------------------- Nhà bán

export type SellerList = Ok<operations["listSellers"]>;
export type SellerDetail = Ok<operations["getSellerDetail"]>;
export type ApproveResult = Ok<operations["approveSeller"]>;
export type SuspendResult = Ok<operations["suspendSeller"]>;

export type SellerStatus = NonNullable<
  operations["listSellers"]["parameters"]["query"]
>["status"];

export function listSellers(
  api: ApiClient,
  filter: { status?: SellerStatus; limit?: number } = {},
): Promise<SellerList> {
  return api.get<SellerList>("/api/v1/admin/sellers", {
    status: filter.status,
    limit: filter.limit,
  });
}

export function getSeller(api: ApiClient, id: string): Promise<SellerDetail> {
  return api.get<SellerDetail>(`/api/v1/admin/sellers/${id}`);
}

export function approveSeller(
  api: ApiClient,
  id: string,
  body: { commission_rate_bp: number; notes?: string },
  idempotencyKey?: string,
): Promise<ApproveResult> {
  return api.post<ApproveResult>(
    `/api/v1/admin/sellers/${id}/approve`,
    body,
    { idempotencyKey },
  );
}

export function suspendSeller(
  api: ApiClient,
  id: string,
  body: { reason: string; reason_code: string },
  idempotencyKey?: string,
): Promise<SuspendResult> {
  return api.post<SuspendResult>(
    `/api/v1/admin/sellers/${id}/suspend`,
    body,
    { idempotencyKey },
  );
}

// ---------------------------------------------------------------- Đơn hàng

export type OrderList = Ok<operations["listAdminOrders"]>;
export type OrderDetail = Ok<operations["getAdminOrderDetail"]>;
export type CancelOrderResult = Ok<operations["cancelAdminOrder"]>;

export function listOrders(
  api: ApiClient,
  filter: { order_number?: string; status?: string; limit?: number } = {},
): Promise<OrderList> {
  return api.get<OrderList>("/api/v1/admin/orders", filter);
}

/**
 * Đọc chi tiết đơn.
 *
 * `reason` là BẮT BUỘC — response chứa tên người nhận, số điện thoại và
 * địa chỉ, và mỗi lần gọi ghi một bản ghi vào nhật ký thao tác.
 */
export function getOrder(
  api: ApiClient,
  id: string,
  reason: string,
): Promise<OrderDetail> {
  return api.get<OrderDetail>(`/api/v1/admin/orders/${id}`, { reason });
}

export function cancelOrder(
  api: ApiClient,
  id: string,
  reason: string,
  idempotencyKey?: string,
): Promise<CancelOrderResult> {
  return api.post<CancelOrderResult>(
    `/api/v1/admin/orders/${id}/cancel`,
    { reason },
    { idempotencyKey },
  );
}

// ------------------------------------------------------------ Nhật ký

export type AuditLog = Ok<operations["listAuditLog"]>;

export type AuditResourceType = NonNullable<
  operations["listAuditLog"]["parameters"]["query"]
>["resource_type"];

export function listAuditLog(
  api: ApiClient,
  filter: {
    resource_type?: AuditResourceType;
    resource_id?: string;
    action?: string;
    actor_id?: string;
    from?: string;
    to?: string;
    limit?: number;
    cursor?: string;
  } = {},
): Promise<AuditLog> {
  return api.get<AuditLog>("/api/v1/admin/audit-log", filter);
}
