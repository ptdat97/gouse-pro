"use client";

import type { NhomMau } from "@fc/api-client";
import * as React from "react";

import { NHOM_MAU, nhomMauHienThi } from "@/lib/mau";

/**
 * Bộ lọc theo NHÓM màu.
 *
 * # Vì sao là nút bấm chứ không phải <select>
 *
 * Màu là thứ khách chọn bằng MẮT. Một danh sách thả xuống toàn chữ bắt họ
 * đọc mười bốn dòng rồi dịch "Xanh dương" thành hình ảnh trong đầu — đúng
 * việc mà ô màu làm thay trong một phần mười giây.
 *
 * Và chọn NHIỀU màu phải làm được: khách tìm áo đen-hoặc-trắng là chuyện
 * thường, còn một `<select>` một lựa chọn thì không cho.
 *
 * # Ô màu KHÔNG đứng một mình
 *
 * Mỗi nút có cả ô màu lẫn chữ. Ô màu không kèm chữ là thứ người mù màu
 * không dùng được — và với "Xám" cạnh "Bạc" thì cả người nhìn rõ cũng
 * không phân biệt nổi.
 *
 * `aria-pressed` nói trạng thái bật/tắt cho trình đọc màn hình, vì đây là
 * nút GIỮ trạng thái chứ không phải nút hành động một lần.
 */
export function BoLocMau({
  dangChon,
  onDoi,
}: {
  dangChon: NhomMau[];
  onDoi: (moi: NhomMau[]) => void;
}) {
  const daChon = React.useMemo(() => new Set(dangChon), [dangChon]);

  function bat(m: NhomMau) {
    onDoi(daChon.has(m) ? dangChon.filter((x) => x !== m) : [...dangChon, m]);
  }

  return (
    <section className="loc" aria-labelledby="loc-mau-tieu-de">
      <div className="loc__dau">
        <h2 id="loc-mau-tieu-de" className="loc__tieu-de">
          Màu sắc
        </h2>

        {/*
          Nút xóa chỉ hiện KHI có gì để xóa.
          Một nút "Xóa lọc" luôn hiện trong lúc không có bộ lọc nào là một
          nút không làm gì — và khách bấm thử để xem nó làm gì.
        */}
        {dangChon.length > 0 && (
          <button type="button" className="loc__xoa" onClick={() => onDoi([])}>
            Xóa ({dangChon.length})
          </button>
        )}
      </div>

      <ul className="mau-list">
        {nhomMauHienThi().map((m) => {
          const chon = daChon.has(m);
          return (
            <li key={m}>
              <button
                type="button"
                className={`mau${chon ? " mau--chon" : ""}`}
                aria-pressed={chon}
                onClick={() => bat(m)}
              >
                <span
                  className="mau__o"
                  style={{ background: NHOM_MAU[m].o }}
                  aria-hidden="true"
                />
                {NHOM_MAU[m].nhan}
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
