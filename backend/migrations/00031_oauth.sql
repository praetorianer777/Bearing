-- +goose Up
-- Bearing as an OAuth authorization server for partner apps (oauth.partner-tokens@1), as Stator and Armature are:
-- an app registers itself (RFC 7591) and waits, pending, until an administrator allows it by connecting it once;
-- then anybody signed in connects it on a consent page, by authorization code with PKCE. What the person allows is a
-- grant: an api_token row of theirs, its access token and refresh token rotated together, acting at the MCP
-- endpoint only. The token endpoint and the consent page act for nobody and run as bearing_admin; a person reads and
-- revokes their own grants as bearing_app.

CREATE TABLE oauth_client (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    name          text NOT NULL CONSTRAINT oauth_client_name_length CHECK (char_length(btrim(name)) BETWEEN 1 AND 60),
    -- read alone is an app that may only read; write lets it do what its person may. read is always among them.
    scopes        text[] NOT NULL CONSTRAINT oauth_client_known_scopes
                  CHECK (scopes <@ ARRAY['read', 'write']::text[] AND 'read' = ANY (scopes)),
    status        text NOT NULL DEFAULT 'pending' CONSTRAINT oauth_client_status_known CHECK (status IN ('pending', 'allowed')),
    redirect_uris text[] NOT NULL CONSTRAINT oauth_client_redirects CHECK (cardinality(redirect_uris) BETWEEN 1 AND 5),
    client_uri    text NOT NULL DEFAULT '',
    -- none is a public client, which proves itself with PKCE alone and keeps no secret.
    auth_method   text NOT NULL CONSTRAINT oauth_client_auth_method_known
                  CHECK (auth_method IN ('client_secret_basic', 'client_secret_post', 'none')),
    -- Only the SHA-256 of the bearing_ocs_ secret, shown once.
    secret_hash   bytea CONSTRAINT oauth_client_secret_is_sha256 CHECK (octet_length(secret_hash) = 32),
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT oauth_client_secret_by_method CHECK ((auth_method = 'none') = (secret_hash IS NULL))
);

CREATE INDEX oauth_client_pending_idx ON oauth_client (created_at) WHERE status = 'pending';

-- A token somebody acts through. A grant, with client_id, holds its current access token in token_hash and its
-- refresh token in refresh_hash; expires_at is when the refresh token runs out unless used, access_expires_at when
-- the access token does. One grant per person and app, gone with the app or the person.
CREATE TABLE api_token (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id        uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    name              text NOT NULL CONSTRAINT api_token_name_length CHECK (char_length(btrim(name)) BETWEEN 1 AND 100),
    scopes            text[] NOT NULL CONSTRAINT api_token_known_scopes
                      CHECK (scopes <@ ARRAY['read', 'write']::text[] AND 'read' = ANY (scopes)),
    token_hash        bytea NOT NULL CONSTRAINT api_token_token_hash_key UNIQUE
                      CONSTRAINT api_token_token_is_sha256 CHECK (octet_length(token_hash) = 32),
    expires_at        timestamptz NOT NULL,
    client_id         uuid REFERENCES oauth_client (id) ON DELETE CASCADE,
    refresh_hash      bytea CONSTRAINT api_token_refresh_hash_key UNIQUE,
    access_expires_at timestamptz,
    mcp_only          boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT api_token_grant_whole CHECK (
        (client_id IS NULL AND refresh_hash IS NULL AND access_expires_at IS NULL AND NOT mcp_only)
        OR (client_id IS NOT NULL AND octet_length(refresh_hash) = 32 AND access_expires_at IS NOT NULL))
);

CREATE INDEX api_token_account_idx ON api_token (account_id);
CREATE UNIQUE INDEX api_token_one_grant_idx ON api_token (account_id, client_id) WHERE client_id IS NOT NULL;

-- Refresh tokens already traded: one presented again ends its grant, as Stator and Armature have no grace window
-- either (RFC 9700 section 4.14.2).
CREATE TABLE oauth_rotated_refresh (
    refresh_hash bytea PRIMARY KEY CONSTRAINT oauth_rotated_refresh_is_sha256 CHECK (octet_length(refresh_hash) = 32),
    grant_id     uuid NOT NULL REFERENCES api_token (id) ON DELETE CASCADE,
    rotated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oauth_rotated_refresh_grant_idx ON oauth_rotated_refresh (grant_id);

-- An authorization code, used once; a code used again ends the grant it made.
CREATE TABLE oauth_code (
    code_hash    bytea PRIMARY KEY CONSTRAINT oauth_code_is_sha256 CHECK (octet_length(code_hash) = 32),
    client_id    uuid NOT NULL REFERENCES oauth_client (id) ON DELETE CASCADE,
    account_id   uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    scopes       text[] NOT NULL CONSTRAINT oauth_code_known_scopes
                 CHECK (scopes <@ ARRAY['read', 'write']::text[] AND 'read' = ANY (scopes)),
    redirect_uri text NOT NULL,
    challenge    text NOT NULL,
    resource     text NOT NULL DEFAULT '',
    expires_at   timestamptz NOT NULL,
    used_at      timestamptz,
    grant_id     uuid REFERENCES api_token (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oauth_code_expiry_idx ON oauth_code (expires_at);

-- Nothing an app asks for works before an administrator allowed it, whoever writes the row.
-- +goose StatementBegin
CREATE FUNCTION oauth_client_allowed_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = public, pg_temp
AS $$
BEGIN
    IF NEW.client_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM oauth_client WHERE id = NEW.client_id AND status = 'allowed') THEN
        RAISE EXCEPTION 'a connected app is allowed by an administrator before anybody connects it'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'oauth_client_allowed';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER api_token_client_allowed BEFORE INSERT OR UPDATE OF client_id ON api_token
    FOR EACH ROW EXECUTE FUNCTION oauth_client_allowed_guard();
CREATE TRIGGER oauth_code_client_allowed BEFORE INSERT OR UPDATE OF client_id ON oauth_code
    FOR EACH ROW EXECUTE FUNCTION oauth_client_allowed_guard();

-- How many people hold a live grant for an app, for administrators, who see no grant but their own.
-- +goose StatementBegin
CREATE FUNCTION oauth_client_people(client uuid) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT count(*)::integer FROM api_token WHERE client_id = client AND expires_at > now()
$$;
-- +goose StatementEnd

ALTER TABLE oauth_client ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_token ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_rotated_refresh ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_code ENABLE ROW LEVEL SECURITY;

CREATE POLICY oauth_client_admin_role ON oauth_client TO bearing_admin USING (true) WITH CHECK (true);
-- Administrators see the apps and remove them; allowing one is connecting it on the consent page.
CREATE POLICY oauth_client_administrators ON oauth_client FOR SELECT TO bearing_app USING ((SELECT current_actor_is_administrator()));
CREATE POLICY oauth_client_remove ON oauth_client FOR DELETE TO bearing_app USING ((SELECT current_actor_is_administrator()));

CREATE POLICY api_token_admin_role ON api_token TO bearing_admin USING (true) WITH CHECK (true);
-- Somebody sees and revokes their own tokens; only the token endpoint makes or renews a grant.
CREATE POLICY api_token_own ON api_token FOR SELECT TO bearing_app USING (account_id = (SELECT current_actor_id()));
CREATE POLICY api_token_revoke ON api_token FOR DELETE TO bearing_app USING (account_id = (SELECT current_actor_id()));

CREATE POLICY oauth_rotated_refresh_admin_role ON oauth_rotated_refresh TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY oauth_code_admin_role ON oauth_code TO bearing_admin USING (true) WITH CHECK (true);

REVOKE ALL ON oauth_client, api_token, oauth_rotated_refresh, oauth_code FROM bearing_app;
GRANT SELECT, DELETE ON oauth_client TO bearing_app;
-- A person never reads a token's hashes back.
GRANT SELECT (id, account_id, name, scopes, expires_at, client_id, access_expires_at, mcp_only, created_at), DELETE ON api_token TO bearing_app;

-- +goose Down
DROP TABLE oauth_code;
DROP TABLE oauth_rotated_refresh;
DROP TABLE api_token;
DROP TABLE oauth_client;
DROP FUNCTION oauth_client_people(uuid);
DROP FUNCTION oauth_client_allowed_guard();
