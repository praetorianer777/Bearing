-- +goose Up
-- Accounts: the installation itself, the people who sign in to it with a password, the invitations that bring them
-- in and the links that reset a forgotten password. Signing in, setting up and the links run as bearing_admin, before
-- anybody acts; everything else runs as bearing_app, held by the policies below.

CREATE TABLE installation (
    -- One row: the table holds the installation, not a list of them.
    id        boolean PRIMARY KEY DEFAULT true CHECK (id),
    name      text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    set_up_at timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE account_role AS ENUM ('administrator', 'editor', 'member', 'reader');

CREATE TABLE account (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    email         citext NOT NULL UNIQUE CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+$' AND length(email) <= 254),
    name          text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    -- NULL for somebody who signs in only through single sign-on.
    password_hash text,
    role          account_role NOT NULL DEFAULT 'member',
    disabled_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invitation (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    email       citext NOT NULL CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+$' AND length(email) <= 254),
    role        account_role NOT NULL,
    -- The link carries the token; only its SHA-256 is kept, so the table cannot be read back into working links.
    token_hash  bytea NOT NULL UNIQUE,
    invited_by  uuid REFERENCES account (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz
);

-- One open invitation per address: inviting again replaces it.
CREATE UNIQUE INDEX invitation_open_email ON invitation (email) WHERE accepted_at IS NULL;

CREATE TABLE password_reset (
    token_hash bytea PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

CREATE INDEX password_reset_account ON password_reset (account_id);

-- Whether the person the transaction acts for administers the installation. It reads account as its owner, so a
-- policy on account can ask it without asking itself.
-- +goose StatementBegin
CREATE FUNCTION current_actor_is_administrator() RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT EXISTS (
        SELECT 1 FROM account
        WHERE id = current_actor_id() AND role = 'administrator' AND disabled_at IS NULL
    )
$$;
-- +goose StatementEnd

-- The app role never reads a password hash: it is granted every column of account but that one.
REVOKE SELECT ON account FROM bearing_app;
GRANT SELECT (id, email, name, role, disabled_at, created_at, updated_at) ON account TO bearing_app;

ALTER TABLE installation ENABLE ROW LEVEL SECURITY;
ALTER TABLE account ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitation ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset ENABLE ROW LEVEL SECURITY;

CREATE POLICY installation_admin_role ON installation TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY installation_read ON installation FOR SELECT TO bearing_app USING (true);
CREATE POLICY installation_rename ON installation FOR UPDATE TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

CREATE POLICY account_admin_role ON account TO bearing_admin USING (true) WITH CHECK (true);
-- Everybody signed in sees who else is; administrators change accounts.
CREATE POLICY account_read ON account FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY account_insert ON account FOR INSERT TO bearing_app WITH CHECK (current_actor_is_administrator());
CREATE POLICY account_update ON account FOR UPDATE TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

CREATE POLICY invitation_admin_role ON invitation TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY invitation_administrators ON invitation TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

-- Resets are read and written only on the way in, as bearing_admin.
CREATE POLICY password_reset_admin_role ON password_reset TO bearing_admin USING (true) WITH CHECK (true);

-- +goose StatementBegin
CREATE FUNCTION touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER account_touch BEFORE UPDATE ON account FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE password_reset;
DROP TABLE invitation;
DROP TABLE account;
DROP TABLE installation;
DROP FUNCTION touch_updated_at();
DROP FUNCTION current_actor_is_administrator();
DROP TYPE account_role;
