"use client";

import {
  getMyBalance,
  isApiError,
  listMySettlements,
  type MyBalance,
  type MySettlements,
} from "@fc/api-client";
import { Alert, Badge, Table, type Column } from "@fc/ui";
import * as React from "react";

import { Shell } from "@/components/shell";
import { dateTime, money, shortId } from "@/lib/format";
import { settlementStatusLabel, settlementTone } from "@/lib/status";
import { useSession } from "@/lib/session";

type Settlement = NonNullable<MySettlements["data"]>[number];

/**
 * Tiền của tôi — số dư và các đợt đối soát.
 *
 * # Vì sao màn hình này quan trọng hơn vẻ ngoài của nó
 *
 * Đặc tả nói thẳng: *"đối soát không minh bạch là nguyên nhân tranh chấp
 * lớn nhất giữa nền tảng và nhà bán"*. Backend tính đúng từ lâu và không ai
 * nhìn thấy — nhà bán chỉ biết tiền chưa về, không biết vì sao.
 *
 * # Một trang, hai câu hỏi
 *
 *	"Tôi có bao nhiêu?"        → số dư, theo trạng thái
 *	"Bao giờ tôi nhận được?"   → các đợt đối soát, mới nhất trước
 *
 * Tách thành hai trang sẽ buộc nhà bán tự ghép hai con số lại — và đó
 * đúng là việc màn hình này tồn tại để làm thay họ.
 */
export default function MoneyPage() {
  return (
    <Shell>
      <Money />
    </Shell>
  );
}

function Money() {
  const { api } = useSession();
  const [balance, setBalance] = React.useState<MyBalance | null>(null);
  const [rows, setRows] = React.useState<Settlement[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let huy = false;

    async function tai() {
      setLoading(true);
      setError(null);
      try {
        // Gọi SONG SONG: hai câu hỏi độc lập nhau, và nối tiếp chỉ làm
        // trang chậm gấp đôi mà không đổi lấy gì.
        const [sd, ds] = await Promise.all([
          getMyBalance(api),
          listMySettlements(api),
        ]);
        if (huy) return;
        setBalance(sd);
        setRows(ds.data ?? []);
      } catch (e) {
        if (huy) return;
        setError(isApiError(e) ? e.message : "Không tải được dữ liệu tiền");
      } finally {
        if (!huy) setLoading(false);
      }
    }

    void tai();
    return () => {
      huy = true;
    };
  }, [api]);

  if (loading) return <p>Đang tải…</p>;

  return (
    <div>
      <h1>Tiền của tôi</h1>
      {error && <Alert tone="danger">{error}</Alert>}

      {balance && <BalanceCards b={balance} />}
      <Settlements rows={rows} />
    </div>
  );
}

/**
 * Số dư — chỉ hiện những ô CÓ NGHĨA hôm nay.
 *
 * Backend trả năm trạng thái, ba trong đó luôn bằng 0 vì chưa được mô hình
 * hóa (`processing`, `on_hold`, `reserve_held` — xem `balanceJSON`).
 *
 * Hiện đủ năm ô thì nhà bán thấy ba ô "0 ₫" và không có cách nào biết đó
 * là "bạn không bị giữ đồng nào" hay "chỗ này hỏng". Hiện hai ô thật, rồi
 * NÓI RA ba cái còn lại, trả lời được cả hai.
 */
function BalanceCards({ b }: { b: MyBalance }) {
  return (
    <section className="panel">
      <div className="balance">
        <Card
          nhan="Chờ đến hạn"
          tien={money(b.pending)}
          giaiThich="Đơn đã giao, còn trong thời hạn đổi trả. Chuyển sang rút được khi hết hạn."
        />
        <Card
          nhan="Rút được"
          tien={money(b.available)}
          giaiThich="Đã hết hạn đổi trả. Sẽ nằm trong đợt đối soát kế tiếp."
        />
      </div>

      <p className="muted">
        Nền tảng hiện <strong>không giữ</strong> khoản nào của bạn: chưa có
        cơ chế giữ tiền do tranh chấp, cũng chưa có chính sách giữ bảo đảm.
        Khi có, chúng sẽ hiện ngay tại đây.
      </p>
    </section>
  );
}

function Card({
  nhan,
  tien,
  giaiThich,
}: {
  nhan: string;
  tien: string;
  giaiThich: string;
}) {
  return (
    <div className="balance__card">
      <div className="balance__label">{nhan}</div>
      <div className="balance__amount">{tien}</div>
      <div className="balance__hint muted">{giaiThich}</div>
    </div>
  );
}

/**
 * Các đợt đối soát.
 *
 * # Cột "Bị trừ" KHÔNG được giấu đi
 *
 * `deficit_amount` là phần nhà bán đang nợ, trừ khỏi số thực chi. Bỏ cột
 * này thì bảng gọn hơn — và mỗi đợt có `net < gross` sẽ thành một cuộc gọi
 * hỏi "sao tháng này ít thế". Chỉ hiện khi khác 0, để bảng không đầy những
 * số 0 ở các kỳ bình thường.
 */
function Settlements({ rows }: { rows: Settlement[] }) {
  const coTru = rows.some((r) => (r.deficit_amount?.amount ?? 0) > 0);

  const columns: Column<Settlement>[] = [
    {
      header: "Kỳ",
      cell: (r) => (
        <>
          {dateTime(r.period_start)} → {dateTime(r.period_end)}
          <br />
          <span className="muted">{shortId(r.id)}</span>
        </>
      ),
    },
    {
      header: "Trạng thái",
      cell: (r) => (
        <Badge tone={settlementTone(r.status)}>
          {settlementStatusLabel(r.status)}
        </Badge>
      ),
    },
    { header: "Tổng", numeric: true, cell: (r) => money(r.gross_amount) },
    ...(coTru
      ? [
          {
            header: "Bị trừ",
            numeric: true,
            cell: (r: Settlement) => {
              const x = r.deficit_amount?.amount ?? 0;
              if (x === 0) return <span className="muted">—</span>;
              // Dấu trừ đặt ở GIAO DIỆN, không ở dữ liệu: backend trả số
              // dương và gọi nó là "phần bị trừ". Một số âm trong dữ liệu
              // là chỗ rất dễ bị cộng nhầm ở phía sau.
              return <span className="deficit">−{money(r.deficit_amount)}</span>;
            },
          },
        ]
      : []),
    {
      header: "Thực nhận",
      numeric: true,
      cell: (r) => <strong>{money(r.net_amount)}</strong>,
    },
  ];

  return (
    <section>
      <h2>Các đợt đối soát</h2>
      <Table
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        empty={
          <>
            Chưa có đợt đối soát nào. Đợt đầu tiên được tạo khi có khoản
            đầu tiên hết thời hạn đổi trả.
          </>
        }
      />
      {coTru && (
        <p className="muted">
          <strong>Bị trừ</strong> là phần còn nợ từ kỳ trước — thường do
          hoàn hàng vượt doanh thu kỳ đó. Khoản nợ chuyển sang kỳ sau chứ
          không bao giờ chuyển tiền âm.
        </p>
      )}
    </section>
  );
}
