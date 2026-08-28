DROP INDEX IF EXISTS variant_attributes_gin;
UPDATE variant SET attributes = attributes - 'color_family'
 WHERE attributes ? 'color_family';
