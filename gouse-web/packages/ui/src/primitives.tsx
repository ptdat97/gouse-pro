/**
 * Component cơ bản dùng chung.
 *
 * # Ba ràng buộc của package này (design-system.md mục 9)
 *
 *   1. KHÔNG chứa logic nghiệp vụ — nghiệp vụ ở backend
 *   2. KHÔNG gọi API — nhận dữ liệu qua props
 *   3. KHÔNG dùng màu/khoảng cách trực tiếp — luôn qua token
 *
 * Ràng buộc 2 quan trọng hơn vẻ ngoài: component gọi API sẽ khó test, khó
 * dùng lại, và làm mờ ranh giới giữa trình bày và lấy dữ liệu.
 *
 * # Khả năng tiếp cận là mặc định, không phải tùy chọn
 *
 * Nhãn liên kết với input, trạng thái focus rõ, modal bẫy focus — đảm bảo ở
 * tầng component để không phụ thuộc vào việc từng lập trình viên có nhớ hay
 * không.
 */
import * as React from "react";

// ---------------------------------------------------------------- Button

type ButtonVariant = "primary" | "secondary" | "danger";

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  loading?: boolean;
}

export function Button({
  variant = "secondary",
  loading = false,
  disabled,
  children,
  className,
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      // Vô hiệu hóa khi đang gửi: bấm lần hai tạo request thứ hai, và với
      // thao tác tài chính đó là một bút toán thừa.
      disabled={disabled || loading}
      data-variant={variant}
      className={["fc-btn", className].filter(Boolean).join(" ")}
      // Báo cho trình đọc màn hình biết nút đang bận, không chỉ "mờ đi".
      aria-busy={loading || undefined}
    >
      {loading ? "Đang xử lý…" : children}
    </button>
  );
}

// ----------------------------------------------------------------- Field

export interface FieldProps {
  label: string;
  htmlFor: string;
  /** Thông báo lỗi. Được liên kết với input qua aria-describedby. */
  error?: string;
  hint?: string;
  children: React.ReactNode;
}

/**
 * Field bọc một input kèm nhãn và thông báo lỗi.
 *
 * Nhãn LUÔN liên kết với input qua `htmlFor`, và lỗi liên kết qua
 * `aria-describedby` — người dùng trình đọc màn hình nghe được lỗi thay vì
 * chỉ thấy viền đỏ.
 */
export function Field({ label, htmlFor, error, hint, children }: FieldProps) {
  const errorId = `${htmlFor}-error`;
  const hintId = `${htmlFor}-hint`;

  return (
    <div className="fc-field">
      <label htmlFor={htmlFor} className="fc-field__label">
        {label}
      </label>

      {React.isValidElement(children)
        ? React.cloneElement(children as React.ReactElement<Record<string, unknown>>, {
            id: htmlFor,
            "aria-invalid": error ? true : undefined,
            "aria-describedby":
              [error ? errorId : null, hint ? hintId : null]
                .filter(Boolean)
                .join(" ") || undefined,
          })
        : children}

      {hint && !error && (
        <p id={hintId} className="fc-field__hint">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} className="fc-field__error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

export type InputProps = React.InputHTMLAttributes<HTMLInputElement>;

export function Input(props: InputProps) {
  return <input {...props} className="fc-input" />;
}

export type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement>;

export function Textarea(props: TextareaProps) {
  return <textarea {...props} className="fc-input fc-input--multiline" />;
}

export type SelectProps = React.SelectHTMLAttributes<HTMLSelectElement>;

export function Select(props: SelectProps) {
  return <select {...props} className="fc-input" />;
}

// ----------------------------------------------------------------- Badge

type BadgeTone = "neutral" | "success" | "warning" | "danger" | "info";

/**
 * Badge hiển thị trạng thái.
 *
 * KHÔNG chỉ dùng màu để truyền đạt trạng thái — chữ luôn có mặt. Khoảng 8%
 * nam giới bị mù màu đỏ-lục, và "chấm đỏ" với "chấm xanh" là hai chấm xám
 * giống nhau với họ.
 */
export function Badge({
  tone = "neutral",
  children,
}: {
  tone?: BadgeTone;
  children: React.ReactNode;
}) {
  return (
    <span className="fc-badge" data-tone={tone}>
      {children}
    </span>
  );
}

// ----------------------------------------------------------------- Table

export interface Column<T> {
  header: string;
  /** Nội dung ô. Nhận cả hàng để dựng ô phức tạp. */
  cell: (row: T) => React.ReactNode;
  /** Căn phải cho cột số — mắt so sánh số theo hàng đơn vị. */
  numeric?: boolean;
}

export interface TableProps<T> {
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  /** Hiển thị khi không có dữ liệu. Rỗng thì dùng câu mặc định. */
  empty?: React.ReactNode;
  onRowClick?: (row: T) => void;
}

export function Table<T>({
  columns,
  rows,
  rowKey,
  empty,
  onRowClick,
}: TableProps<T>) {
  if (rows.length === 0) {
    return <div className="fc-empty">{empty ?? "Không có dữ liệu"}</div>;
  }

  return (
    <div className="fc-table-wrap">
      <table className="fc-table">
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.header} data-numeric={c.numeric || undefined}>
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={rowKey(row)}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              // Hàng bấm được phải dùng được bằng BÀN PHÍM. Không có phần
              // này thì cả trang không điều hướng được nếu thiếu chuột.
              tabIndex={onRowClick ? 0 : undefined}
              role={onRowClick ? "button" : undefined}
              onKeyDown={
                onRowClick
                  ? (e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        onRowClick(row);
                      }
                    }
                  : undefined
              }
              data-clickable={onRowClick ? true : undefined}
            >
              {columns.map((c) => (
                <td key={c.header} data-numeric={c.numeric || undefined}>
                  {c.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ----------------------------------------------------------------- Alert

/**
 * Alert hiển thị thông báo quan trọng.
 *
 * `role="alert"` cho tone danger/warning: trình đọc màn hình đọc ngay,
 * không đợi người dùng di chuyển tới. Với cảnh báo "thao tác không hoàn
 * tác", đợi là quá muộn.
 */
export function Alert({
  tone = "info",
  title,
  children,
}: {
  tone?: BadgeTone;
  title?: string;
  children: React.ReactNode;
}) {
  const urgent = tone === "danger" || tone === "warning";
  return (
    <div className="fc-alert" data-tone={tone} role={urgent ? "alert" : "status"}>
      {title && <strong className="fc-alert__title">{title}</strong>}
      <div>{children}</div>
    </div>
  );
}
