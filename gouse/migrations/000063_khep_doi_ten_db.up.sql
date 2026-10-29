-- KHÉP việc đổi tên định danh ở tầng DATABASE.
--
-- # Vì sao còn một lượt nữa sau 000062
--
-- 000062 đổi tên BẢNG và CỘT, nhưng PostgreSQL KHÔNG đổi theo tên của
-- những thứ sinh ra từ chúng: ràng buộc NOT NULL/CHECK và chỉ mục giữ
-- nguyên tên cũ. Hệ quả không phải chuyện thẩm mỹ — nó là một chú thích
-- nêu tên ĐÃ CHẾT, dạng tệ nhất, vì nó nằm trong THÔNG BÁO LỖI:
--
--   ERROR: null value in column "key" violates not-null constraint
--   constraint "ops_config_khoa_not_null"
--
-- Người đọc lỗi đi tìm cột `khoa` và không có cột nào tên thế. 14 ràng
-- buộc của `size_observation` còn mang tên bảng cũ `quan_sat_size`.
--
-- Phép rà: đọc `information_schema` + `pg_constraint` + `pg_indexes` rồi
-- lọc tên tiếng Việt. Trước migration này: 28 tên. Sau: 0.

-- ------------------------------------------------------- cột còn tiếng Việt
--
-- `cho_thanh_toan` lọt khỏi 000062. Nó là cờ khóa giao hàng cho đơn trả
-- trước — xem 000044 và `domain.AwaitingPayment`.
ALTER TABLE fulfillment_order RENAME COLUMN cho_thanh_toan TO awaiting_payment;

ALTER TABLE fulfillment_order
    RENAME CONSTRAINT fulfillment_order_cho_thanh_toan_not_null
                   TO fulfillment_order_awaiting_payment_not_null;

ALTER INDEX fulfillment_order_cho_thanh_toan_idx
    RENAME TO fulfillment_order_awaiting_payment_idx;

COMMENT ON COLUMN fulfillment_order.awaiting_payment IS
    'TRUE = đơn trả trước chưa thu được tiền, KHÓA mọi bước giao hàng. '
    'Mở khóa một chiều khi event order.paid tới. Xem ADR-0018 phần A2.';

-- --------------------------------------------- ràng buộc mang tên cột cũ
--
-- `ops_config`: 000062 đổi năm cột (khoa→key, gia_tri→value, sua_luc→
-- updated_at, sua_boi→updated_by, ly_do→reason) mà không đổi bảy ràng buộc.
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_khoa_check        TO ops_config_key_check;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_khoa_not_null     TO ops_config_key_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_gia_tri_not_null  TO ops_config_value_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_sua_luc_not_null  TO ops_config_updated_at_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_sua_boi_check     TO ops_config_updated_by_check;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_sua_boi_not_null  TO ops_config_updated_by_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_ly_do_check       TO ops_config_reason_check;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_ly_do_not_null    TO ops_config_reason_not_null;

-- ------------------------------------------- ràng buộc mang tên BẢNG cũ
--
-- `quan_sat_size` → `size_observation` ở 000062. Mười bốn ràng buộc, kể cả
-- khoá chính, còn tên cũ.
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_pkey                   TO size_observation_pkey;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_id_not_null            TO size_observation_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_customer_id_check      TO size_observation_customer_id_check;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_customer_id_not_null   TO size_observation_customer_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_brand_id_check         TO size_observation_brand_id_check;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_brand_id_not_null      TO size_observation_brand_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_size_check             TO size_observation_size_check;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_size_not_null          TO size_observation_size_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_ket_qua_check          TO size_observation_outcome_check;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_ket_qua_not_null       TO size_observation_outcome_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_nguon_loai_check       TO size_observation_source_type_check;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_nguon_loai_not_null    TO size_observation_source_type_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_nguon_id_check         TO size_observation_source_id_check;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_nguon_id_not_null      TO size_observation_source_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT quan_sat_size_quan_sat_luc_not_null  TO size_observation_observed_at_not_null;

-- ---------------------------------------------------- chỉ mục còn tiếng Việt
ALTER INDEX demand_signal_gom_idx            RENAME TO demand_signal_rollup_idx;
ALTER INDEX payment_intent_theo_nha_cung_cap RENAME TO payment_intent_by_provider_idx;
