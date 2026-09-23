"use client";

import {
  getMyBalance,
  getMySettlement,
  isApiError,
  listMySettlements,
  type MyBalance,
  type MySettlement,
  type MySettlements,
} from "@fc/api-client";
import { Alert, Badge, Button, Table, type Column } from "@fc/ui";
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
  // Số dư chờ ÂM là trạng thái THẬT, không phải lỗi hiển thị: hoàn hàng
  // của những đơn đã chuyển sang rút được sẽ trừ vào phần đang chờ. Hiện
  // một số âm trần trụi thì nhà bán nghĩ hệ thống hỏng; giấu đi thì họ
  // không hiểu vì sao đợt đối soát kế tiếp bị trừ.
  const dangNo = (b.pending?.amount ?? 0) < 0;

  return (
    <section className="panel">
      <div className="balance">
        <Card
          nhan="Chờ đến hạn"
          tien={money(b.pending)}
          giaiThich={
            dangNo
              ? "Đang ÂM: phần hoàn hàng lớn hơn phần bán mới trong kỳ. Khoản âm này sẽ trừ vào đợt đối soát kế tiếp, không thu lại bằng tiền mặt."
              : "Đơn đã giao, còn trong thời hạn đổi trả. Chuyển sang rút được khi hết hạn."
          }
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
  const [mo, setMo] = React.useState<string | null>(null);
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
        onRowClick={(r) => setMo(mo === r.id ? null : r.id!)}
        empty={
          <>
            Chưa có đợt đối soát nào. Đợt đầu tiên được tạo khi có khoản
            đầu tiên hết thời hạn đổi trả.
          </>
        }
      />
      {mo && <ChiTiet id={mo} onDong={() => setMo(null)} />}

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

/**
 * Chi tiết một đợt — TỪNG KHOẢN làm nên số tiền.
 *
 * # Vì sao trang này cần tồn tại
 *
 * Đặc tả viết thẳng: nhà bán phải xem được *"từng dòng cấu thành số tiền —
 * đối soát không minh bạch là nguyên nhân tranh chấp lớn nhất giữa nền
 * tảng và nhà bán"*.
 *
 * Một con số tổng không đối chiếu được với bất cứ thứ gì nhà bán tự ghi.
 * Mỗi dòng ở đây trỏ tới một ĐƠN THỰC HIỆN — chính là đơn họ thấy ở màn
 * hình "Việc cần làm" — nên họ đối chiếu được từng đơn một.
 *
 * # Tải khi MỞ, không tải sẵn
 *
 * Một đợt có thể gồm hàng trăm đơn. Nhồi hết vào danh sách thì trang tiền
 * — thứ nhà bán mở hằng ngày chỉ để xem số dư — kéo theo dữ liệu của mọi
 * đợt trong lịch sử.
 */
function ChiTiet({ id, onDong }: { id: string; onDong: () => void }) {
  const { api } = useSession();
  const [d, setD] = React.useState<MySettlement["settlement"] | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let huy = false;
    setLoading(true);
    setError(null);
    setD(null);
    getMySettlement(api, id)
      .then((r) => {
        if (!huy) setD(r.settlement);
      })
      .catch((e) => {
        if (!huy) setError(isApiError(e) ? e.message : "Không tải được chi tiết");
      })
      .finally(() => {
        if (!huy) setLoading(false);
      });
    return () => {
      huy = true;
    };
  }, [api, id]);

  const dong = d?.lines ?? [];

  return (
    <section className="panel">
      <h3>Đợt {shortId(id)} gồm những gì</h3>
      {loading && <p>Đang tải…</p>}
      {error && <Alert tone="danger">{error}</Alert>}

      {!loading && !error && dong.length === 0 && (
        <p className="muted">Đợt này không có khoản nào.</p>
      )}

      {dong.length > 0 && (
        <>
          <ul className="lines">
            {dong.map((l) => (
              <li key={l.id} className="line">
                <div>
                  {/*
                    Mã đơn thực hiện là thứ nhà bán ĐỐI CHIẾU ĐƯỢC — họ
                    thấy đúng mã ấy ở màn hình "Việc cần làm".
                  */}
                  <span className="mono">{shortId(l.reference_id)}</span>
                  <div className="muted">
                    Chuyển sang rút được {dateTime(l.released_at)}
                  </div>
                </div>
                <div>{money(l.amount)}</div>
              </li>
            ))}
          </ul>
          <p className="muted">
            {dong.length} khoản · cộng lại bằng cột <strong>Tổng</strong> ở
            bảng trên.
          </p>
        </>
      )}

      <p>
        <Button variant="secondary" onClick={onDong}>
          Đóng
        </Button>
      </p>
    </section>
  );
}
