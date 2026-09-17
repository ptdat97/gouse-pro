-- Dựng lại y như migration 000015, để đi xuống rồi đi lên không đổi schema.
--
-- KHÔNG khôi phục dữ liệu, vì chưa bao giờ có dòng nào: bảng không có
-- đường ghi trong suốt vòng đời của nó.

CREATE TABLE notification_preference (
    user_id  TEXT NOT NULL,
    channel  TEXT NOT NULL CHECK (channel IN ('EMAIL', 'SMS', 'PUSH', 'IN_APP')),

    -- Chỉ MARKETING và SOCIAL tắt được. TRANSACTIONAL không có ở đây.
    category TEXT NOT NULL CHECK (category IN ('MARKETING', 'SOCIAL')),

    enabled BOOLEAN NOT NULL DEFAULT true,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, channel, category)
);
