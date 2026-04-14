DELETE FROM default_templates WHERE type = 'injectable';

ALTER TABLE default_templates DROP COLUMN type;
