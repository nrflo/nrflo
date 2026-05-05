ALTER TABLE users ADD COLUMN system INTEGER NOT NULL DEFAULT 0;
UPDATE users SET system = 1 WHERE id = 'usr_admin_seed';
