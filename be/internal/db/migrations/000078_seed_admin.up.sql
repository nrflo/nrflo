INSERT OR IGNORE INTO users (id, email, display_name, password_hash, role, status, must_change_password, created_at, updated_at)
VALUES (
    'usr_admin_seed',
    'admin@nrflo.com',
    'Admin',
    '$argon2id$v=19$m=65536,t=3,p=2$849PsyNeHcJ2HIioUrr9Kw$lC7qhb6GEB8eJrov0Y8lY28mqopVtAA8Sl+opWdxVac',
    'admin',
    'active',
    1,
    '2024-01-01T00:00:00Z',
    '2024-01-01T00:00:00Z'
);
