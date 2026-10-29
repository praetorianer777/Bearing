-- +goose Up
-- Foundation: the two runtime roles, what they may reach, and whom a transaction acts for, which every later
-- table's row level security policies read.

-- IDs come from uuidv7(), which Postgres has built in from version 18 on.
-- +goose StatementBegin
DO $$
BEGIN
    IF current_setting('server_version_num')::int < 180000 THEN
        RAISE EXCEPTION 'Bearing needs PostgreSQL 18 or newer, and this server runs %. Upgrade it and migrate again.',
            current_setting('server_version');
    END IF;
END;
$$;
-- +goose StatementEnd

CREATE EXTENSION IF NOT EXISTS citext;

-- bearing_app is subject to every policy; bearing_admin is exempt by policy rather than by being a superuser, so the
-- exemption is visible in the schema. A deployment that created them beforehand, with a password to log in, keeps
-- them.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'bearing_app') THEN
        CREATE ROLE bearing_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'bearing_admin') THEN
        CREATE ROLE bearing_admin NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
END;
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO bearing_app, bearing_admin;

-- Everything later migrations create is granted to the runtime roles as it is made, so a new table is never
-- accidentally unreachable; the policies decide what each role sees of it.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO bearing_app, bearing_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO bearing_app, bearing_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT EXECUTE ON FUNCTIONS TO bearing_app, bearing_admin;

-- The person the transaction acts for, from app.user_id. NULL binds the app role to nothing at all, so a
-- transaction that names nobody fails closed.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION current_actor_id() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE
AS $$
    SELECT NULLIF(current_setting('app.user_id', true), '')::uuid
$$;
-- +goose StatementEnd

-- The token or grant the person acts through, from app.via; NULL is a session.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION current_via_id() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE
AS $$
    SELECT NULLIF(current_setting('app.via', true), '')::uuid
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS current_via_id();
DROP FUNCTION IF EXISTS current_actor_id();
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE EXECUTE ON FUNCTIONS FROM bearing_app, bearing_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE USAGE, SELECT ON SEQUENCES FROM bearing_app, bearing_admin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM bearing_app, bearing_admin;
REVOKE USAGE ON SCHEMA public FROM bearing_app, bearing_admin;
