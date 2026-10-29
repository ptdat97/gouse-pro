-- Đảo 000064. Đổi lại TỪNG HÀNG, không chỉ đổi ràng buộc.
ALTER TABLE email_verification_token DROP CONSTRAINT email_verification_token_id_check;

UPDATE email_verification_token
   SET id = 'evt_' || substring(id FROM 5)
 WHERE id LIKE 'emt\_%';

ALTER TABLE email_verification_token
    ADD CONSTRAINT email_verification_token_id_check
    CHECK (id LIKE 'evt\_%' AND length(id) = 30);

ALTER TABLE size_observation DROP CONSTRAINT size_observation_outcome_check;

UPDATE size_observation SET outcome = CASE outcome
    WHEN 'PURCHASED' THEN 'DA_MUA'
    WHEN 'TIGHT'     THEN 'CHAT'
    WHEN 'LOOSE'     THEN 'RONG'
    ELSE outcome
END;

ALTER TABLE size_observation
    ADD CONSTRAINT size_observation_outcome_check
    CHECK (outcome IN ('DA_MUA', 'CHAT', 'RONG'));
