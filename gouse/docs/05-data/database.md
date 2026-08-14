# Database

## 1. Lựa chọn: PostgreSQL

**Quyết định:** dùng một PostgreSQL duy nhất cho MVP.

### Lý do

| Yêu cầu của nền tảng | PostgreSQL đáp ứng |
|---|---|
| Giao dịch ACID cho tài chính | Có, đầy đủ |
| Ràng buộc toàn vẹn dữ liệu | `CHECK`, `UNIQUE`, khóa ngoại |
| Khóa lạc quan | Hỗ trợ tốt |
| Chỉ mục có điều kiện | Có |
| Dữ liệu linh hoạt | JSONB |
| Phân vùng bảng lớn | Có |
| Tìm kiếm toàn văn cơ bản | Có (đủ cho MVP) |
| Đội ngũ quen thuộc | Phổ biến, dễ tuyển |

### Vì sao không dùng nhiều loại database từ đầu

```text
Cám dỗ:
    - Redis cho cache
    - Elasticsearch cho tìm kiếm
    - ClickHouse cho analytics
    - MongoDB cho catalog linh hoạt

Vấn đề:
    - Mỗi loại thêm chi phí vận hành, sao lưu, giám sát
    - Nhất quán giữa các hệ thống trở thành vấn đề
    - Đội nhỏ không đủ năng lực vận hành tốt cả bốn

Nguyên tắc: thêm hạ tầng khi ĐO ĐƯỢC nhu cầu, không phải phỏng đoán.
```

Xem nguyên tắc P15 tại [../00-overview/principles.md](../00-overview/principles.md).

---

## 2. Lộ trình bổ sung hạ tầng

| Hạ tầng | Khi nào thêm | Dấu hiệu cần |
|---|---|---|
| Object storage | **Ngay từ MVP** | Ảnh/video không lưu trong database |
| Cache | Phase 2 | Có điểm nóng đo được, database tải cao |
| Chỉ mục tìm kiếm | Phase 2 | Tìm kiếm SQL chậm hoặc thiếu tính năng |
| Lưu trữ chuỗi thời gian | Phase 3 | Ghi analytics ảnh hưởng database chính |
| Bản sao chỉ đọc | Phase 2–3 | Truy vấn báo cáo ảnh hưởng giao dịch |

**Object storage là ngoại lệ duy nhất cần từ đầu** — lưu ảnh trong database là sai lầm rõ ràng, không cần đo cũng biết.

---

## 3. Tách schema theo module

> **Trạng thái triển khai (14/08/2026): CHƯA áp dụng.** Toàn bộ 20 migration
> hiện dùng schema `public` mặc định. Mục này mô tả phương án đã thiết kế,
> chưa phải hiện trạng — xem [ghi chú cuối mục](#vì-sao-chưa-tách-schema).

```sql
CREATE SCHEMA commerce;      -- catalog, product, pricing, cart, checkout, order
CREATE SCHEMA marketplace;   -- offer, seller
CREATE SCHEMA inventory;
CREATE SCHEMA financial;     -- payment, ledger
CREATE SCHEMA growth;        -- creator, content, affiliate, campaign
CREATE SCHEMA supply;        -- supply-chain, procurement, manufacturing, quality, warehouse
CREATE SCHEMA platform;      -- identity, notification, analytics
```

### Lợi ích

```text
1. Ranh giới module hiển thị ngay trong database
2. Cấp quyền theo schema → kiểm soát truy cập rõ ràng
3. Khi tách service: chuyển cả schema, dễ hơn nhiều
4. Dễ phát hiện vi phạm: truy vấn JOIN qua schema khác nhìn thấy ngay
```

### Cấp quyền

```sql
-- Mỗi module có role riêng (nếu muốn thực thi nghiêm ngặt)
GRANT USAGE ON SCHEMA inventory TO app_inventory;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA inventory TO app_inventory;
REVOKE ALL ON SCHEMA financial FROM app_inventory;
```

**Lưu ý:** cách này mạnh nhưng phức tạp khi vận hành monolith với một kết nối. Có thể bắt đầu bằng schema (rõ ràng về mặt tổ chức) và thêm phân quyền sau nếu cần.

### Vì sao chưa tách schema

Sau khi triển khai 10 module, việc tách schema **chưa được làm**, và đây là
quyết định có ý thức chứ không phải bỏ sót:

```text
Lợi ích mà tách schema đem lại:
    1. Ranh giới hiển thị trong database
    2. Cấp quyền theo schema
    3. Dễ tách service
    4. Dễ phát hiện JOIN vượt ranh giới

Thực tế hiện tại:
    (1) → đã có qua quy ước đặt tên bảng + tài liệu sở hữu dữ liệu
    (2) → monolith một kết nối, một role; phân quyền chưa có tác dụng gì
    (3) → chưa tách service nào; ADR-0009 nói khi nào mới tách
    (4) → archcheck chặn ở tầng import Go, trước khi tới SQL
```

Nói cách khác: ba trong bốn lợi ích **chưa đến kỳ thu hoạch**, còn lợi ích
thứ nhất đã có bằng cách khác rẻ hơn.

**Chi phí nếu làm sớm:** mọi truy vấn phải mang tiền tố schema, `search_path`
trở thành cấu hình phải quản lý, và migration khó đọc hơn — đổi lấy lợi ích
chỉ xuất hiện khi bắt đầu tách service.

**Khi nào làm:** cùng lúc với việc tách service đầu tiên (ADR-0009), vì lúc
đó "chuyển cả schema" mới thật sự là thao tác rẻ hơn "chuyển từng bảng".
Đây là việc **đã ghi nhận**, không phải việc bị quên.

---

## 4. Cấu hình kết nối

```text
Nhóm kết nối (connection pool):
    - Kích thước tối đa: theo số lõi CPU của database, không phải theo số request
    - Quá nhiều kết nối làm database chậm đi, không nhanh hơn
    - Có timeout cho việc lấy kết nối

Tách nhóm kết nối:
    - Nhóm cho request người dùng (ưu tiên cao)
    - Nhóm cho tác vụ nền (giới hạn thấp hơn)
    → tác vụ nền không được chiếm hết kết nối
```

**Nguyên tắc:** tác vụ nền nặng không bao giờ được làm khách hàng không đặt được hàng.

---

## 5. Mức cô lập giao dịch

```text
Mặc định: READ COMMITTED (mặc định của PostgreSQL)

Dùng mức cao hơn khi:
    - Đọc rồi ghi dựa trên giá trị đọc được
    → nhưng ưu tiên dùng khóa lạc quan thay vì tăng mức cô lập
```

### Ví dụ: cập nhật tồn kho

```sql
-- KHÔNG cần SERIALIZABLE, dùng điều kiện nguyên tử
UPDATE inventory_item
SET quantity_available = quantity_available - $qty,
    quantity_reserved  = quantity_reserved + $qty,
    version = version + 1
WHERE id = $id
  AND version = $expected_version
  AND quantity_available >= $qty;
```

Câu lệnh này an toàn ở mức `READ COMMITTED` vì kiểm tra và cập nhật diễn ra nguyên tử trong một câu lệnh.

Xem [consistency.md](consistency.md).

---

## 6. Sao lưu

Chi tiết tại [../09-operations/backup.md](../09-operations/backup.md). Tóm tắt yêu cầu:

```text
- Sao lưu toàn bộ định kỳ
- Lưu WAL liên tục → khôi phục tới thời điểm bất kỳ
- Kiểm tra khôi phục định kỳ (sao lưu chưa thử khôi phục = không có sao lưu)
- Lưu ở vị trí địa lý khác
```

---

## 7. Bản sao chỉ đọc

```text
Khi nào cần:
    - Truy vấn báo cáo làm chậm giao dịch
    - Dashboard admin quét nhiều dữ liệu

Cách dùng:
    Ghi           → bản chính
    Đọc giao dịch → bản chính (cần dữ liệu mới nhất)
    Báo cáo       → bản sao (chấp nhận trễ vài giây)
```

**Cảnh báo:** không đọc dữ liệu tài chính hoặc tồn kho từ bản sao khi cần ra quyết định — độ trễ sao chép có thể dẫn tới quyết định sai.

---

## 8. Giám sát database

| Chỉ báo | Ngưỡng cảnh báo |
|---|---|
| Truy vấn chậm (> 1s) | > 10/phút |
| Kết nối đang dùng | > 80% giới hạn |
| Độ trễ sao chép | > 10 giây |
| Kích thước database | Theo dõi xu hướng |
| Tỷ lệ trúng cache | < 95% |
| Giao dịch bị khóa lâu | > 5 giây |
| Bảng phình do dead tuple | > 20% |
| Chỉ mục không được dùng | Rà soát định kỳ |

---

## 9. Chống mất mát dữ liệu tài chính

Dữ liệu tài chính có yêu cầu cao hơn phần còn lại:

```text
1. Sổ cái bất biến — không sửa, không xóa (RULE ở tầng database)
2. Sao lưu thường xuyên hơn
3. Kiểm tra tính toàn vẹn định kỳ:
   - Mọi ledger_entry có Σ DEBIT = Σ CREDIT?
   - balance_snapshot khớp với tổng bút toán?
4. Đối chiếu với sao kê PSP và ngân hàng
5. Lưu trữ dài hạn theo quy định kế toán
```

Bất kỳ sai lệch nào ở kiểm tra số 3 là **sự cố nghiêm trọng**, không phải sai số chấp nhận được.

---

## 10. Tài liệu liên quan

- [data-model.md](data-model.md) — thiết kế schema
- [consistency.md](consistency.md) — nhất quán và giao dịch
- [../09-operations/backup.md](../09-operations/backup.md) — sao lưu chi tiết
- [../09-operations/deployment.md](../09-operations/deployment.md) — triển khai
