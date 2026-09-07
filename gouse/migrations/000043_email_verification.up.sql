-- XÁC MINH EMAIL (P3-15) — mở đường gộp lịch sử đơn vãng lai.
--
-- # Vấn đề nó giải quyết
--
-- Khách đặt hàng vãng lai bằng email X, sau đó KHÔNG đăng ký được tài
-- khoản bằng chính email đó: hồ sơ vãng lai chứa lịch sử mua hàng và địa
-- chỉ nhà, nên gắn nó vào một tài khoản vừa tạo nghĩa là bất kỳ ai biết
-- email người khác đều đọc được những thứ đó.
--
-- Từ chối là quyết định ĐÚNG khi chưa có cách chứng minh quyền sở hữu
-- email. Bảng này là cách đó.
CREATE TABLE email_verification_token (
    id TEXT PRIMARY KEY CHECK (id LIKE 'evt\_%' AND length(id) = 30),

    user_id TEXT NOT NULL REFERENCES "user" (id),

    -- token_hash: BĂM chứ không lưu nguyên văn.
    --
    -- Cùng lý do với `session.refresh_token_hash`: rò rỉ database mà token
    -- lưu nguyên văn nghĩa là kẻ tấn công xác minh được email của người
    -- khác, và từ đó gộp được hồ sơ vãng lai của họ về tài khoản mình.
    --
    -- Băm SHA-256 chứ không bcrypt, có chủ ý: token do `crypto/rand` sinh
    -- ra với đủ entropy nên không có gì để dò: bcrypt ở đây chỉ làm mỗi
    -- lần xác minh chậm đi mà không thêm một chút an toàn nào.
    token_hash TEXT NOT NULL UNIQUE,

    -- Email TẠI THỜI ĐIỂM phát token.
    --
    -- Lưu lại chứ không đọc từ `user` lúc xác minh: người dùng đổi email
    -- giữa chừng thì token cũ phải chết, không được xác minh cho email
    -- mới. Không có cột này thì một token gửi tới địa chỉ cũ lại xác minh
    -- được địa chỉ mới — tức là bỏ qua chính bước đang làm.
    email TEXT NOT NULL CHECK (length(trim(email)) > 0),

    expires_at TIMESTAMPTZ NOT NULL,

    -- used_at khác NULL nghĩa là ĐÃ dùng. KHÔNG xóa hàng: cần biết token
    -- được dùng lúc nào khi điều tra một lần gộp hồ sơ đáng ngờ.
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Tra theo băm là đường DUY NHẤT lúc xác minh.
CREATE INDEX email_verification_token_theo_user
    ON email_verification_token (user_id, created_at DESC);

-- Token còn hiệu lực của một người: dùng để chặn phát tràn lan.
CREATE INDEX email_verification_token_con_hieu_luc
    ON email_verification_token (user_id)
    WHERE used_at IS NULL;
