-- QUAN SÁT SIZE của khách — đầu vào của gợi ý size.
--
-- # Vì sao lưu QUAN SÁT chứ không lưu kết luận
--
-- Không có cột "size gợi ý đã tính sẵn". Lý do giống hệt lý do sổ cái lưu
-- bút toán chứ không lưu số dư: quy tắc gợi ý SẼ đổi, và đổi quy tắc không
-- được làm mất dữ liệu đã quan sát. Tính lại từ quan sát thì được; dựng lại
-- quan sát từ một kết luận cũ thì không.
--
-- # Vì sao có `brand_id`
--
-- Size M của hai thương hiệu KHÔNG bằng nhau. Đó là sự thật của ngành thời
-- trang, không phải hạn chế kỹ thuật — gợi ý xuyên thương hiệu cần bảng quy
-- đổi số đo, thứ thuộc `BODY_MEASUREMENTS` và chưa có.
CREATE TABLE quan_sat_size (
    id BIGSERIAL PRIMARY KEY,

    customer_id TEXT NOT NULL CHECK (length(trim(customer_id)) > 0),
    brand_id    TEXT NOT NULL CHECK (length(trim(brand_id)) > 0),

    -- size là NHÃN như khách thấy: "M", "38", "Free".
    --
    -- Lưu nguyên văn chứ không quy đổi: quy đổi cần bảng số đo của từng
    -- thương hiệu, và đoán bừa một thang chung là cách chắc chắn nhất để
    -- gợi ý sai.
    size TEXT NOT NULL CHECK (length(trim(size)) > 0),

    -- ket_qua là điều khách NÓI RA về size đó.
    --
    --   DA_MUA   đã mua và KHÔNG trả vì lý do size
    --   CHAT     trả hàng vì SIZE_TOO_SMALL  → lần sau nên lên một size
    --   RONG     trả hàng vì SIZE_TOO_LARGE  → lần sau nên xuống một size
    --
    -- DA_MUA là tín hiệu YẾU: khách giữ hàng có thể vì vừa, cũng có thể vì
    -- ngại trả. CHAT và RONG mạnh hơn hẳn vì khách chủ động bỏ công nói ra.
    ket_qua TEXT NOT NULL CHECK (ket_qua IN ('DA_MUA', 'CHAT', 'RONG')),

    -- Nguồn quan sát, dùng để chống trùng khi event được phát lại.
    nguon_loai TEXT NOT NULL CHECK (length(trim(nguon_loai)) > 0),
    nguon_id   TEXT NOT NULL CHECK (length(trim(nguon_id)) > 0),

    quan_sat_luc TIMESTAMPTZ NOT NULL
);

-- CHỐNG TRÙNG: outbox giao ÍT NHẤT MỘT LẦN, nên cùng một event tới hai lần
-- là chuyện bình thường. Không có chỉ mục này thì một lần mua bị đếm nhiều
-- lần và lấn át các quan sát khác.
CREATE UNIQUE INDEX quan_sat_size_nguon_idx
    ON quan_sat_size (customer_id, brand_id, size, ket_qua, nguon_loai, nguon_id);

-- Đường đọc: "khách này đã quan sát gì ở thương hiệu này", mới nhất trước.
CREATE INDEX quan_sat_size_khach_idx
    ON quan_sat_size (customer_id, brand_id, quan_sat_luc DESC);
