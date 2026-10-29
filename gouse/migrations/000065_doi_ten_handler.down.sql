-- Đảo 000065. Phải đi cùng việc đảo mã Go: tên trong `event_processed` và
-- tên `Handler.Name()` trả về PHẢI khớp, không thì event phát lại bị xử lý
-- hai lần.
UPDATE event_processed SET handler = 'recommendation.ghi_quan_sat_size'      WHERE handler = 'recommendation.record_size_observation';
UPDATE event_processed SET handler = 'promotion.giai_phong_luot_khi_huy_don' WHERE handler = 'promotion.release_use_on_order_cancelled';
UPDATE event_processed SET handler = 'promotion.ghi_luot_dung_khi_hoan_tat'  WHERE handler = 'promotion.record_use_on_checkout_completed';
UPDATE event_processed SET handler = 'payment.ghi_chi_phi_hang_van_chuyen'   WHERE handler = 'payment.record_carrier_cost_on_handover';
UPDATE event_processed SET handler = 'payment.ghi_thu_tien_khi_don_da_tra'   WHERE handler = 'payment.record_payment_on_order_paid';
UPDATE event_processed SET handler = 'fulfillment.huy_khi_don_bi_huy'        WHERE handler = 'fulfillment.cancel_on_order_cancelled';
UPDATE event_processed SET handler = 'fulfillment.mo_khoa_khi_da_tra_tien'   WHERE handler = 'fulfillment.unlock_on_order_paid';
UPDATE event_processed SET handler = 'affiliate.mo_khoa_khi_het_han_doi_tra' WHERE handler = 'affiliate.unlock_on_return_window_passed';
UPDATE event_processed SET handler = 'affiliate.dao_khi_huy_don'             WHERE handler = 'affiliate.reverse_on_order_cancelled';
UPDATE event_processed SET handler = 'affiliate.dao_khi_huy_don_thuc_hien'   WHERE handler = 'affiliate.reverse_on_fulfillment_cancelled';
UPDATE event_processed SET handler = 'affiliate.quy_cong_khi_da_tra_tien'    WHERE handler = 'affiliate.attribute_on_order_paid';
