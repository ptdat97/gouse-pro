"use client";

import {
  getMyPerformance,
  isApiError,
  type KyHieuSuat,
  type MyPerformance,
} from "@fc/api-client";
import { Alert, Badge, Button } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import {
  chiSoGiaTri,
  chiSoNguong,
  chiSoNhan,
  chiSoTone,
  chiSoTrangThaiNhan,
  chiSoYNghia,
  KY,
} from "@/lib/hieu-suat";
import { useSession } from "@/lib/session";

/**
 * Hiệu suất gian hàng.
 *
 * # Màn hình này tồn tại để KHÔNG phải là hộp đen
 *
 * Đặc tả viết: *"Seller cần hiểu vì sao mình không thắng buy box và cần làm
 * gì để cải thiện. Mô hình chấm điểm hộp đen tạo tranh chấp không giải
 * quyết được và cảm giác bất công."*
 *
 * Nên trang này hiện BA thứ mà một bảng điểm thường giấu:
 *
 *	ngưỡng          con số đang dùng để chấm, ngay cạnh giá trị
 *	cỡ mẫu          "tỷ lệ hủy 0%" của bao nhiêu đơn — không có nó thì
 *	                con số không kiểm chứng được
 *	chỉ số CHƯA chấm  kèm lý do, gồm cả "chưa đủ mẫu, cần 10 có 0"
 *
 * Mục thứ ba là mục dễ bỏ nhất và đáng giữ nhất: một chỉ số biến mất không
 * lời giải thích tạo ra đúng cảm giác bất công mà cả endpoint sinh ra để
 * tránh.
 */
export default function PerformancePage() {
  return (
    <Shell>
      <Performance />
    </Shell>
  );
}

function Performance() {
  const { api } = useSession();
  const [ky, setKy] = React.useState<KyHieuSuat>("LAST_30_DAYS");
  const [data, setData] = React.useState<MyPerformance | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let huy = false;
    setLoading(true);
    setError(null);
    getMyPerformance(api, ky)
      .then((d) => {
        if (!huy) setData(d);
      })
      .catch((e) => {
        if (!huy) {
          setError(isApiError(e) ? e.message : "Không tải được chỉ số");
        }
      })
      .finally(() => {
        if (!huy) setLoading(false);
      });
    return () => {
      huy = true;
    };
  }, [api, ky]);

  const metrics = data?.metrics ?? [];
  const chuaDo = data?.not_measured ?? [];

  return (
    <div>
      <h1>Hiệu suất gian hàng</h1>

      <p className="toolbar">
        {KY.map((k) => (
          <Button
            key={k.ma}
            variant={k.ma === ky ? "primary" : "secondary"}
            disabled={loading}
            onClick={() => setKy(k.ma)}
          >
            {k.nhan}
          </Button>
        ))}
      </p>

      {error && <Alert tone="danger">{error}</Alert>}
      {loading && <p>Đang tải…</p>}

      {!loading && data && (
        <>
          {/*
            Thông điệp của backend đứng TRƯỚC bảng số.
            Đặc tả đòi trả lời cả "cần làm gì", và một danh sách con số
            không trả lời vế đó. Backend nêu chỉ số TỆ NHẤT kèm việc cần
            làm — đó là câu nhà bán cần đọc đầu tiên.
          */}
          {data.impact?.message && (
            <Alert tone={metrics.some((m) => m.status !== "GOOD") ? "warning" : "info"}>
              {data.impact.message}
            </Alert>
          )}

          <CoSoChamDiem
            sampleSize={data.sample_size}
            slaGio={data.shipping_sla_hours}
          />

          {metrics.length > 0 && (
            <section>
              <h2>Chỉ số đang được chấm</h2>
              {metrics.map((m) => (
                <ChiSo key={m.name} m={m} />
              ))}
            </section>
          )}

          <ChuaCham rows={chuaDo} />
        </>
      )}
    </div>
  );
}

/**
 * Cơ sở chấm điểm — hai con số làm mọi tỷ lệ ở trên kiểm chứng được.
 *
 * Đặt TRƯỚC bảng chỉ số, không phải chú thích cuối trang: nhà bán cần biết
 * mẫu là bao nhiêu trước khi đọc một tỷ lệ, không phải sau.
 */
function CoSoChamDiem({
  sampleSize,
  slaGio,
}: {
  sampleSize: number | undefined;
  slaGio: number | undefined;
}) {
  return (
    <section className="panel">
      <h2>Chấm trên cái gì</h2>
      <ul className="lines">
        <li className="line">
          <div>
            Số đơn trong kỳ
            <div className="muted">
              Mẫu dùng để tính mọi tỷ lệ bên dưới.
            </div>
          </div>
          <div>
            <strong>{(sampleSize ?? 0).toLocaleString("vi-VN")}</strong>
          </div>
        </li>
        <li className="line">
          <div>
            Hạn bàn giao
            <div className="muted">
              Tính từ lúc đơn thực hiện được tạo. Đây là thước đo quyết định
              một đơn có “đúng hạn” hay không.
            </div>
          </div>
          <div>
            <strong>{slaGio ?? "—"} giờ</strong>
          </div>
        </li>
      </ul>
    </section>
  );
}

type Metric = NonNullable<MyPerformance["metrics"]>[number];

function ChiSo({ m }: { m: Metric }) {
  return (
    <section className="panel">
      <p>
        <strong>{chiSoNhan(m.name)}</strong>{" "}
        <Badge tone={chiSoTone(m.status)}>
          {chiSoTrangThaiNhan(m.status)}
        </Badge>
      </p>

      <ul className="lines">
        <li className="line">
          <div>Giá trị của bạn</div>
          <div>
            <strong>{chiSoGiaTri(m.name, m.value)}</strong>
          </div>
        </li>
        {/*
          Ngưỡng nằm NGAY CẠNH giá trị, không nằm ở trang chính sách.
          Đây là toàn bộ khác biệt giữa "bạn bị chấm 3 sao" và "bạn ở 4%,
          ngưỡng là ≤ 3%".
        */}
        <li className="line">
          <div>Ngưỡng đang áp dụng</div>
          <div>{chiSoNguong(m.name, m.threshold)}</div>
        </li>
      </ul>

      {chiSoYNghia(m.name) && <p className="muted">{chiSoYNghia(m.name)}</p>}
    </section>
  );
}

type ChuaDoRow = NonNullable<MyPerformance["not_measured"]>[number];

/**
 * Chỉ số CHƯA chấm được — phần dễ bỏ nhất của trang này.
 *
 * Bỏ nó đi thì trang gọn hơn và nhà bán mất khả năng phân biệt hai chuyện
 * hoàn toàn khác nhau: "chỉ số này không tồn tại" với "chỉ số này có,
 * nhưng bạn chưa đủ đơn để nó có nghĩa".
 *
 * KHÔNG tách hai loại bằng cách dò chuỗi lý do: API không gắn nhãn loại,
 * và đoán theo nội dung câu chữ là dựng một hợp đồng ngầm mà backend không
 * biết mình đã ký.
 */
function ChuaCham({ rows }: { rows: ChuaDoRow[] }) {
  if (rows.length === 0) return null;

  return (
    <section>
      <h2>Chưa chấm được, và vì sao</h2>
      <p className="muted">
        Những chỉ số dưới đây không tính vào đánh giá gian hàng của bạn.
        Chúng tôi nói rõ lý do thay vì bỏ trống.
      </p>
      <ul className="lines">
        {rows.map((c) => (
          <li key={c.name} className="line">
            <div>
              <strong>{chiSoNhan(c.name)}</strong>
              <div className="muted">{c.reason}</div>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
