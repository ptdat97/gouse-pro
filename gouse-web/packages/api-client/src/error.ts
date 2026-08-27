/**
 * Lỗi API có cấu trúc, khớp `components/common.yaml#/schemas/Error`.
 *
 * # Xử lý theo `code`, KHÔNG parse `message`
 *
 * `message` có thể đổi và có thể đa ngôn ngữ. Một nhánh `if` so khớp chuỗi
 * tiếng Việt sẽ im lặng ngừng hoạt động vào ngày ai đó sửa chính tả.
 */

/** Chi tiết lỗi của một trường, dùng cho form. */
export interface FieldError {
  field: string;
  code: string;
  message: string;
}

/** Thân lỗi mà backend trả về. */
interface ErrorBody {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
    field_errors?: FieldError[];
  };
  request_id: string;
}

/**
 * ApiError là lỗi từ backend, đã có cấu trúc.
 *
 * Giữ `requestId` vì đó là thứ nhân viên hỗ trợ cần khi khách báo lỗi —
 * nó tra ngược được toàn bộ chuỗi từ request tới bút toán.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: Record<string, unknown>;
  readonly fieldErrors: FieldError[];
  readonly requestId: string;

  constructor(status: number, body: ErrorBody) {
    super(body.error.message);
    this.name = "ApiError";
    this.status = status;
    this.code = body.error.code;
    this.details = body.error.details ?? {};
    this.fieldErrors = body.error.field_errors ?? [];
    this.requestId = body.request_id;
  }

  /**
   * Chưa xác thực — đăng nhập lại CÓ ích.
   *
   * Khác `isForbidden`: đó là đã đăng nhập nhưng không đủ quyền, và đăng
   * nhập lại vô ích. Nhầm hai thứ này khiến client rơi vào vòng lặp làm
   * mới token vô vọng.
   */
  get isUnauthorized(): boolean {
    return this.status === 401;
  }

  /** Đã xác thực nhưng không đủ quyền — đăng nhập lại VÔ ích. */
  get isForbidden(): boolean {
    return this.status === 403;
  }

  get isNotFound(): boolean {
    return this.status === 404;
  }

  /** Xung đột trạng thái: thao tác không hợp lệ với trạng thái hiện tại. */
  get isConflict(): boolean {
    return this.status === 409;
  }
}

/** Lỗi mạng hoặc server không trả JSON đúng đặc tả. */
export class NetworkError extends Error {
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.name = "NetworkError";
  }
}

/** Phân biệt lỗi API có cấu trúc với lỗi mạng. */
export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

/**
 * Mã lỗi đã biết, để `switch` không cần chuỗi rời rạc.
 *
 * Danh sách này KHÔNG đầy đủ và không cần đầy đủ: server có thể thêm mã
 * mới trong cùng phiên bản API, nên mọi `switch` PHẢI có nhánh mặc định.
 */
export const ErrorCode = {
  ValidationFailed: "VALIDATION_FAILED",
  Unauthorized: "UNAUTHORIZED",
  Forbidden: "FORBIDDEN",
  NotFound: "NOT_FOUND",
  Conflict: "CONFLICT",
  RateLimitExceeded: "RATE_LIMIT_EXCEEDED",
  InternalError: "INTERNAL_ERROR",
  LedgerEntryUnbalanced: "LEDGER_ENTRY_UNBALANCED",
  OrderNotCancellable: "ORDER_NOT_CANCELLABLE",
  InsufficientInventory: "INSUFFICIENT_INVENTORY",
} as const;
