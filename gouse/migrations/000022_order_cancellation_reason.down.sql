-- Gỡ lý do hủy đơn.
--
-- CẢNH BÁO: cột này là dữ liệu nghiệp vụ, không phải bộ nhớ đệm. Gỡ nó là
-- MẤT HẲN lý do hủy của mọi đơn — không có nguồn nào dựng lại được.
ALTER TABLE "order"
    DROP COLUMN cancellation_reason;
