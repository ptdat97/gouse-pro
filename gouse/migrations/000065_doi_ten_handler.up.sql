-- Tên handler: 11 tên còn tiếng Việt, và chúng là DỮ LIỆU.
--
-- # Vì sao đây không phải một phép đổi tên như 000063
--
-- `Handler.Name()` được GHI XUỐNG DATABASE làm khoá idempotency:
--
--   eventbus/dispatcher.go:208
--     INSERT INTO event_processed (event_id, handler) VALUES ($1, $2)
--     ON CONFLICT DO NOTHING
--
-- Đổi tên trong Go mà KHÔNG đổi dữ liệu nghĩa là mọi cặp (event, handler)
-- đã xử lý không còn khớp. Một event phát lại — chuyện bình thường của
-- giao hàng ít-nhất-một-lần — sẽ được xử lý LẠI: bút toán ghi hai lần,
-- lượt dùng mã trừ hai lần, hoa hồng quy công hai lần.
--
-- Đo trên môi trường phát triển: `event_processed` có 42.362 hàng, trong đó
-- 169 hàng thuộc 11 handler dưới đây.
--
-- # Vì sao vẫn nên đổi
--
-- 9 tên đã tiếng Anh, 11 tên tiếng Việt. Đó đúng là tập NỬA ANH NỬA VIỆT
-- mà mục 2 của quy-uoc-dat-ten.md gọi là tệ hơn cả hai thái cực: người đọc
-- log phải biết CẢ HAI quy ước để đoán tên handler thứ 21.
--
-- Mỗi tên mới là chữ thường của ĐÚNG kiểu Go đã mang tên ấy, nên không có
-- lựa chọn nào phải cân: `RecordSizeObservation` →
-- `recommendation.record_size_observation`.

UPDATE event_processed SET handler = 'affiliate.attribute_on_order_paid'          WHERE handler = 'affiliate.quy_cong_khi_da_tra_tien';
UPDATE event_processed SET handler = 'affiliate.reverse_on_fulfillment_cancelled' WHERE handler = 'affiliate.dao_khi_huy_don_thuc_hien';
UPDATE event_processed SET handler = 'affiliate.reverse_on_order_cancelled'       WHERE handler = 'affiliate.dao_khi_huy_don';
UPDATE event_processed SET handler = 'affiliate.unlock_on_return_window_passed'   WHERE handler = 'affiliate.mo_khoa_khi_het_han_doi_tra';
UPDATE event_processed SET handler = 'fulfillment.unlock_on_order_paid'           WHERE handler = 'fulfillment.mo_khoa_khi_da_tra_tien';
UPDATE event_processed SET handler = 'fulfillment.cancel_on_order_cancelled'      WHERE handler = 'fulfillment.huy_khi_don_bi_huy';
UPDATE event_processed SET handler = 'payment.record_payment_on_order_paid'       WHERE handler = 'payment.ghi_thu_tien_khi_don_da_tra';
UPDATE event_processed SET handler = 'payment.record_carrier_cost_on_handover'    WHERE handler = 'payment.ghi_chi_phi_hang_van_chuyen';
UPDATE event_processed SET handler = 'promotion.record_use_on_checkout_completed' WHERE handler = 'promotion.ghi_luot_dung_khi_hoan_tat';
UPDATE event_processed SET handler = 'promotion.release_use_on_order_cancelled'   WHERE handler = 'promotion.giai_phong_luot_khi_huy_don';
UPDATE event_processed SET handler = 'recommendation.record_size_observation'     WHERE handler = 'recommendation.ghi_quan_sat_size';
