ALTER TABLE return_line
    DROP COLUMN IF EXISTS reason_code,
    DROP COLUMN IF EXISTS reason_detail;
