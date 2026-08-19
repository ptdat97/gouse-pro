"use client";

import * as Dialog from "@radix-ui/react-dialog";
import * as React from "react";

import { Alert, Button, Field, Textarea } from "./primitives";

/**
 * ReasonDialog — hộp thoại xác nhận cho THAO TÁC NHẠY CẢM.
 *
 * Ba lớp bảo vệ, theo docs/08-frontend/admin.md mục 3:
 *
 *   1. Cảnh báo rõ về tính không hoàn tác
 *   2. Lý do bắt buộc, có độ dài tối thiểu
 *   3. Hiển thị TÁC ĐỘNG DỰ KIẾN trước khi xác nhận
 *
 * # Kiểm tra ở đây CHỈ là trải nghiệm
 *
 * Backend vẫn từ chối lý do rác — người dùng gọi API trực tiếp được, bỏ
 * qua hoàn toàn giao diện. Việc kiểm tra ở đây để họ biết ngay thay vì gửi
 * đi rồi nhận lỗi.
 *
 * # Vì sao dùng Radix Dialog
 *
 * Bẫy focus, đóng bằng Escape, khóa cuộn nền, `aria-modal` — đều là thứ
 * làm đúng rất tốn công và làm sai thì người dùng bàn phím kẹt lại trong
 * trang nền.
 */

/** Khớp `minLength: 20` trong đặc tả và `minReasonLen` ở platform/audit. */
export const MIN_REASON_LENGTH = 20;

/**
 * Các mẫu lý do rác, khớp `junkReasons` ở internal/platform/audit.
 *
 * Danh sách này KHÔNG chặn được người cố tình, và không cố làm thế — mục
 * tiêu là nhắc người đang vội gõ cho đủ ký tự.
 */
const JUNK_PATTERNS = ["test", "fix", "abc", "xxx", "asdf", "1234", "..."];

/** Trả thông báo lỗi, hoặc null nếu lý do dùng được. */
export function validateReason(reason: string): string | null {
  const trimmed = reason.trim();

  if (trimmed.length < MIN_REASON_LENGTH) {
    return `Cần tối thiểu ${MIN_REASON_LENGTH} ký tự, hiện có ${trimmed.length}`;
  }

  const lower = trimmed.toLowerCase();
  for (const junk of JUNK_PATTERNS) {
    if (lower.split(junk).join("") === "") {
      return "Lý do phải có nội dung thật — đây là thứ duy nhất giải thích được thao tác này sau sáu tháng";
    }
  }

  return null;
}

export interface ReasonDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;

  title: string;

  /**
   * Cảnh báo về tính không hoàn tác. Bỏ trống nếu thao tác đảo được.
   */
  warning?: React.ReactNode;

  /**
   * Tác động dự kiến, hiển thị TRƯỚC khi xác nhận.
   *
   * Ví dụ "sẽ ẩn 142 offer". Người vận hành cần biết hậu quả trước khi
   * bấm, không phải sau.
   */
  impact?: React.ReactNode;

  confirmLabel: string;
  /** Nhãn nút nên nói rõ HÀNH ĐỘNG, không phải "OK". */
  confirmTone?: "primary" | "danger";

  /** Lỗi từ server, hiển thị trong hộp thoại thay vì làm mất dữ liệu đã nhập. */
  serverError?: string | null;

  submitting?: boolean;
  onConfirm: (reason: string) => void;
}

export function ReasonDialog({
  open,
  onOpenChange,
  title,
  warning,
  impact,
  confirmLabel,
  confirmTone = "danger",
  serverError,
  submitting = false,
  onConfirm,
}: ReasonDialogProps) {
  const [reason, setReason] = React.useState("");
  const [touched, setTouched] = React.useState(false);

  // Xóa nội dung khi đóng: mở lại cho thao tác KHÁC mà còn lý do cũ là
  // cách dễ nhất để ghi nhầm lý do vào nhật ký.
  React.useEffect(() => {
    if (!open) {
      setReason("");
      setTouched(false);
    }
  }, [open]);

  const error = validateReason(reason);
  const showError = touched && error !== null;

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fc-dialog__overlay" />
        <Dialog.Content className="fc-dialog" aria-describedby={undefined}>
          <Dialog.Title className="fc-dialog__title">{title}</Dialog.Title>

          {warning && (
            <Alert tone="danger" title="Thao tác này KHÔNG THỂ hoàn tác">
              {warning}
            </Alert>
          )}

          {impact && (
            <div className="fc-dialog__impact">
              <strong>Tác động dự kiến</strong>
              {impact}
            </div>
          )}

          <form
            onSubmit={(e) => {
              e.preventDefault();
              setTouched(true);
              if (!error) onConfirm(reason.trim());
            }}
          >
            <Field
              label="Lý do (bắt buộc)"
              htmlFor="reason"
              error={showError ? error : undefined}
              hint={`Tối thiểu ${MIN_REASON_LENGTH} ký tự. Nội dung này được ghi vào nhật ký thao tác.`}
            >
              <Textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                onBlur={() => setTouched(true)}
                rows={3}
                autoFocus
              />
            </Field>

            {serverError && (
              <Alert tone="danger">{serverError}</Alert>
            )}

            <div className="fc-dialog__actions">
              <Dialog.Close asChild>
                <Button type="button" disabled={submitting}>
                  Hủy bỏ
                </Button>
              </Dialog.Close>
              <Button
                type="submit"
                variant={confirmTone}
                loading={submitting}
                // KHÔNG vô hiệu hóa khi lý do chưa đạt: nút mờ không giải
                // thích vì sao. Cho bấm rồi hiện lỗi cụ thể ngay dưới ô.
              >
                {confirmLabel}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
