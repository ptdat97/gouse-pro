-- Bỏ bảng token xác minh email.
--
-- MẤT DỮ LIỆU: mọi token đang chờ xác minh biến mất, và người dùng phải
-- yêu cầu lại. `user.email_verified_at` KHÔNG bị đụng — ai đã xác minh thì
-- vẫn xác minh.
DROP TABLE IF EXISTS email_verification_token;
