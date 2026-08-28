DROP INDEX IF EXISTS return_line_cho_kiem_dinh;
ALTER TABLE return_line DROP CONSTRAINT IF EXISTS return_line_fail_needs_note;
ALTER TABLE return_line
    DROP COLUMN IF EXISTS inspection,
    DROP COLUMN IF EXISTS inspection_note,
    DROP COLUMN IF EXISTS inspected_at;
