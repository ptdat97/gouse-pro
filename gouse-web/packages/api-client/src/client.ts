import { ApiError, NetworkError } from "./error";

/**
 * Client gọi API Go backend.
 *
 * # Access token nằm TRONG BỘ NHỚ, không phải localStorage
 *
 * localStorage đọc được bằng JavaScript, nên một lỗ hổng XSS duy nhất là
 * mất tài khoản. Token trong bộ nhớ mất khi tải lại trang — và đó chính là
 * lý do refresh token nằm ở cookie httpOnly, thứ JavaScript không đọc được.
 *
 * # Client này KHÔNG chứa logic nghiệp vụ
 *
 * Không tính tiền, không quyết định trạng thái, không kiểm tra quyền thật.
 * Nó chỉ gửi request và chuyển lỗi thành kiểu dùng được. Xem nguyên tắc P4
 * ở docs/00-overview/principles.md.
 */

export interface ClientOptions {
  /** Địa chỉ gốc của API, ví dụ http://localhost:8080. */
  baseUrl: string;

  /** Ngôn ngữ gửi kèm mọi request. */
  locale?: string;

  /**
   * Gọi khi phiên hết hạn và làm mới cũng thất bại.
   *
   * Nơi gọi thường dùng để chuyển hướng về trang đăng nhập. Client KHÔNG
   * tự chuyển hướng: nó không biết ứng dụng đang dùng bộ định tuyến nào.
   */
  onSessionExpired?: () => void;
}

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | undefined>;

  /**
   * Khóa idempotency cho lệnh ghi.
   *
   * Bỏ trống thì client tự sinh. Truyền vào khi muốn một khóa GẮN VỚI Ý
   * ĐỊNH của người dùng — ví dụ giữ nguyên khóa qua các lần bấm lại nút,
   * để hai lần bấm không tạo hai bút toán.
   */
  idempotencyKey?: string;

  /** Bỏ qua việc tự làm mới token. Dùng cho chính endpoint refresh. */
  skipRefresh?: boolean;

  /**
   * Header bổ sung cho riêng lời gọi này.
   *
   * Dùng cho những thứ KHÔNG phải danh tính chung của client — ví dụ
   * `X-Guest-Phone` khi khách vãng lai tra đơn: nó chứng minh quyền xem
   * MỘT đơn cụ thể, không phải quyền của cả phiên.
   */
  headers?: Record<string, string>;
}

export class ApiClient {
  private accessToken: string | null = null;
  private refreshing: Promise<boolean> | null = null;

  constructor(private readonly opts: ClientOptions) {}

  /** Đặt access token sau khi đăng nhập hoặc làm mới. */
  setAccessToken(token: string | null): void {
    this.accessToken = token;
  }

  hasSession(): boolean {
    return this.accessToken !== null;
  }

  async get<T>(
    path: string,
    query?: RequestOptions["query"],
    headers?: Record<string, string>,
  ): Promise<T> {
    return this.request<T>(path, { method: "GET", query, headers });
  }

  async post<T>(
    path: string,
    body?: unknown,
    opts?: Pick<RequestOptions, "idempotencyKey">,
  ): Promise<T> {
    return this.request<T>(path, {
      method: "POST",
      body,
      idempotencyKey: opts?.idempotencyKey,
    });
  }

  async patch<T>(
    path: string,
    body?: unknown,
    opts?: Pick<RequestOptions, "idempotencyKey">,
  ): Promise<T> {
    return this.request<T>(path, {
      method: "PATCH",
      body,
      idempotencyKey: opts?.idempotencyKey,
    });
  }

  /**
   * Xóa một tài nguyên.
   *
   * Tên `del` chứ không phải `delete`: `delete` là từ khóa của JavaScript.
   */
  async del<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: "DELETE" });
  }

  private async request<T>(path: string, opts: RequestOptions): Promise<T> {
    const res = await this.send(path, opts);

    // 401 → thử làm mới MỘT lần rồi gửi lại.
    //
    // Chỉ một lần: nếu lần thứ hai vẫn 401 thì phiên đã thật sự hết, và
    // thử tiếp chỉ tạo vòng lặp vô hạn với server.
    if (res.status === 401 && !opts.skipRefresh) {
      const refreshed = await this.refreshOnce();
      if (refreshed) {
        return this.parse<T>(await this.send(path, opts));
      }
      this.accessToken = null;
      this.opts.onSessionExpired?.();
    }

    return this.parse<T>(res);
  }

  private async send(path: string, opts: RequestOptions): Promise<Response> {
    const url = new URL(path, this.opts.baseUrl);
    for (const [k, v] of Object.entries(opts.query ?? {})) {
      if (v !== undefined && v !== "") url.searchParams.set(k, String(v));
    }

    const headers: Record<string, string> = {
      "Accept-Language": this.opts.locale ?? "vi-VN",
    };
    if (this.accessToken) {
      headers["Authorization"] = `Bearer ${this.accessToken}`;
    }
    if (opts.body !== undefined) {
      headers["Content-Type"] = "application/json";
    }

    // Mọi POST/PATCH đều cần Idempotency-Key — backend từ chối nếu thiếu.
    const method = opts.method ?? "GET";
    if (method === "POST" || method === "PATCH") {
      headers["Idempotency-Key"] = opts.idempotencyKey ?? newIdempotencyKey();
    }

    // Header riêng của lời gọi đặt SAU cùng, nhưng KHÔNG ghi đè
    // Authorization: danh tính của phiên không phải thứ mỗi lời gọi tự đổi.
    for (const [k, v] of Object.entries(opts.headers ?? {})) {
      if (k.toLowerCase() !== "authorization") headers[k] = v;
    }

    try {
      return await fetch(url, {
        method,
        headers,
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
        // BẮT BUỘC: refresh token nằm ở cookie httpOnly, không gửi cookie
        // thì không làm mới được phiên.
        credentials: "include",
      });
    } catch (e) {
      throw new NetworkError("Không kết nối được máy chủ", { cause: e });
    }
  }

  /**
   * Làm mới token, gộp các lời gọi đồng thời thành MỘT.
   *
   * Không gộp thì ba request cùng nhận 401 sẽ gọi refresh ba lần; mà mỗi
   * lần refresh THU HỒI token cũ, nên lần thứ hai và thứ ba thất bại và
   * người dùng bị đăng xuất oan.
   */
  private refreshOnce(): Promise<boolean> {
    this.refreshing ??= this.doRefresh().finally(() => {
      this.refreshing = null;
    });
    return this.refreshing;
  }

  private async doRefresh(): Promise<boolean> {
    try {
      const res = await this.send("/api/v1/auth/refresh", {
        method: "POST",
        skipRefresh: true,
      });
      if (!res.ok) return false;

      const data = (await res.json()) as { access_token?: string };
      if (!data.access_token) return false;

      this.accessToken = data.access_token;
      return true;
    } catch {
      return false;
    }
  }

  private async parse<T>(res: Response): Promise<T> {
    if (res.status === 204) return undefined as T;

    const text = await res.text();

    if (!res.ok) {
      // Server luôn trả lỗi đúng đặc tả. Nếu không parse được thì đó là
      // sự cố hạ tầng (proxy trả HTML, timeout) — báo rõ thay vì để lỗi
      // JSON khó hiểu nổi lên tận giao diện.
      try {
        throw new ApiError(res.status, JSON.parse(text));
      } catch (e) {
        if (e instanceof ApiError) throw e;
        throw new NetworkError(`Máy chủ trả lỗi ${res.status}`, { cause: text });
      }
    }

    try {
      return JSON.parse(text) as T;
    } catch (e) {
      throw new NetworkError("Máy chủ trả dữ liệu không hợp lệ", { cause: e });
    }
  }
}

/**
 * Sinh khóa idempotency.
 *
 * Khóa gắn với MỘT LẦN GỬI của client. Muốn gắn với ý định của người dùng
 * (giữ nguyên qua nhiều lần bấm lại) thì truyền `idempotencyKey` vào.
 */
function newIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID().replace(/-/g, "");
  }
  return Math.random().toString(36).slice(2).padEnd(32, "0");
}
