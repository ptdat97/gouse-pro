-- Đảo 000063. Đổi tên là phép đảo được hoàn toàn: không dữ liệu nào mất.
ALTER INDEX payment_intent_by_provider_idx RENAME TO payment_intent_theo_nha_cung_cap;
ALTER INDEX demand_signal_rollup_idx       RENAME TO demand_signal_gom_idx;

ALTER TABLE size_observation RENAME CONSTRAINT size_observation_observed_at_not_null  TO quan_sat_size_quan_sat_luc_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_source_id_not_null    TO quan_sat_size_nguon_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_source_id_check       TO quan_sat_size_nguon_id_check;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_source_type_not_null  TO quan_sat_size_nguon_loai_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_source_type_check     TO quan_sat_size_nguon_loai_check;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_outcome_not_null      TO quan_sat_size_ket_qua_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_outcome_check         TO quan_sat_size_ket_qua_check;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_size_not_null         TO quan_sat_size_size_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_size_check            TO quan_sat_size_size_check;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_brand_id_not_null     TO quan_sat_size_brand_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_brand_id_check        TO quan_sat_size_brand_id_check;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_customer_id_not_null  TO quan_sat_size_customer_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_customer_id_check     TO quan_sat_size_customer_id_check;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_id_not_null           TO quan_sat_size_id_not_null;
ALTER TABLE size_observation RENAME CONSTRAINT size_observation_pkey                  TO quan_sat_size_pkey;

ALTER TABLE ops_config RENAME CONSTRAINT ops_config_reason_not_null     TO ops_config_ly_do_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_reason_check        TO ops_config_ly_do_check;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_updated_by_not_null TO ops_config_sua_boi_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_updated_by_check    TO ops_config_sua_boi_check;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_updated_at_not_null TO ops_config_sua_luc_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_value_not_null      TO ops_config_gia_tri_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_key_not_null        TO ops_config_khoa_not_null;
ALTER TABLE ops_config RENAME CONSTRAINT ops_config_key_check           TO ops_config_khoa_check;

ALTER INDEX fulfillment_order_awaiting_payment_idx RENAME TO fulfillment_order_cho_thanh_toan_idx;
ALTER TABLE fulfillment_order
    RENAME CONSTRAINT fulfillment_order_awaiting_payment_not_null
                   TO fulfillment_order_cho_thanh_toan_not_null;
ALTER TABLE fulfillment_order RENAME COLUMN awaiting_payment TO cho_thanh_toan;
