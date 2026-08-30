-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user_password';
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;

-- Enable and FORCE Row-Level Security on tenant-scoped tables
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

-- Tenant isolation policy checking active tenant session config and status
DROP POLICY IF EXISTS tenant_isolation_users ON users;
CREATE POLICY tenant_isolation_users ON users
    FOR ALL
    TO app_user
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)
        AND EXISTS (SELECT 1 FROM tenants WHERE id = tenant_id AND status = 'active')
    )
    WITH CHECK (
        tenant_id = current_setting('app.current_tenant_id', true)
        AND EXISTS (SELECT 1 FROM tenants WHERE id = tenant_id AND status = 'active')
    );

-- +goose Down
DROP POLICY IF EXISTS tenant_isolation_users ON users;
ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;
