INSERT OR IGNORE INTO users (id, email, display_name, password_hash, role, status, must_change_password, created_at, updated_at)
VALUES (
    'usr_admin_seed',
    'admin',
    'Admin',
    '$argon2id$v=19$m=65536,t=3,p=2$wp1TNUWXyUKgfR4DUO9lNw$I3aPnpvRtOZrJU4VESkpkNfcPpg7sncPAZtxKJZDpm4',
    'admin',
    'active',
    0,
    '2024-01-01T00:00:00Z',
    '2024-01-01T00:00:00Z'
);
