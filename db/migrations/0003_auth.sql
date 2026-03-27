CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    sub TEXT UNIQUE NOT NULL,
    email TEXT,
    provider TEXT NOT NULL DEFAULT 'local',
    password_hash TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    last_login TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS user_tenant_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    PRIMARY KEY (user_id, tenant_id, role)
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    obj TEXT NOT NULL,
    act TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_sub TEXT,
    actor_email TEXT,
    provider TEXT,
    tenant_id UUID,
    action TEXT,
    resource TEXT,
    result TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant ON audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expiry ON refresh_tokens(expires_at);

INSERT INTO tenants (id, name)
VALUES ('00000000-0000-0000-0000-000000000001', 'default')
ON CONFLICT (name) DO NOTHING;

-- Seed default admin user with password 'admin' (change after first login).
INSERT INTO users (id, sub, email, provider, password_hash, status)
VALUES (
    '00000000-0000-0000-0000-00000000000a',
    'admin',
    'admin@example.com',
    'local',
    '$2a$10$ueFOcGRKCWDeOaTwy1hmQ.WjQ70Yu8JJLcl8ZvJprx7HPKArt8ESC',
    'active'
)
ON CONFLICT (sub) DO NOTHING;

INSERT INTO user_tenant_roles (user_id, tenant_id, role)
VALUES (
    '00000000-0000-0000-0000-00000000000a',
    '00000000-0000-0000-0000-000000000001',
    'nopsai-admin'
)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role, tenant_id, name, obj, act)
SELECT 'nopsai-admin', '00000000-0000-0000-0000-000000000001', 'All access', '/*', '.*'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permissions WHERE role = 'nopsai-admin' AND tenant_id = '00000000-0000-0000-0000-000000000001' AND obj = '/*' AND act = '.*'
);
