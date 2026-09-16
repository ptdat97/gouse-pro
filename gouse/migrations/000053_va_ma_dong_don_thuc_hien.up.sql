-- Vá `fulfillment_order_line.order_line_id`: nó đang chứa mã dòng của
-- PHIÊN THANH TOÁN (`cln_…`) chứ không phải của ĐƠN HÀNG (`oln_…`).
--
-- VÌ SAO PHẢI VÁ, KHÔNG CHỈ SỬA CODE
--
-- Bản sửa ở tầng ứng dụng chỉ đúng cho đơn ĐẶT TỪ NAY. Mọi đơn đã có
-- trong database vẫn giữ mã sai, và trường ấy tồn tại để trang chi tiết
-- đơn ghép kiện với dòng hàng — phép ghép khớp 0 dòng trên đơn cũ, tức
-- khách không bao giờ biết món nào đi trong gói nào cho những đơn đó.
--
-- CÁCH GHÉP LẠI: qua offer.
--
-- Dòng phiên và dòng đơn không có khóa chung, nhưng cả hai đều trỏ tới
-- một offer, và một offer chỉ xuất hiện MỘT lần trong giỏ (`cart.AddItem`
-- cộng dồn số lượng vào dòng cũ). Nên cặp (đơn, offer) xác định đúng một
-- dòng đơn.
--
-- Dòng nào không ghép được thì GIỮ NGUYÊN: một mã sai còn lần ngược được,
-- một mã bịa thì không.

UPDATE fulfillment_order_line fol
SET order_line_id = ol.id
FROM fulfillment_order fo
     JOIN checkout c ON c.order_id = fo.order_id
     JOIN order_line ol ON ol.order_id = fo.order_id
     JOIN checkout_line cl
          ON cl.checkout_id = c.id
         AND cl.offer_id = ol.offer_id
WHERE fol.fulfillment_order_id = fo.id
  AND fol.order_line_id = cl.id
  AND fol.order_line_id LIKE 'cln\_%';
