ALTER TABLE default_templates ADD COLUMN default_template TEXT;
UPDATE default_templates SET default_template = template WHERE readonly = 1;
