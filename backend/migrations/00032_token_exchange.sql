-- +goose Up
-- Token exchange (oauth.token-exchange@1, RFC 8693), as Stator and Armature offer it. A partner app is registered by
-- an administrator with the realm client its people sign in through; a person's realm access token, issued to that
-- client with Bearing among its audiences, is traded for a grant of theirs without a consent page. Such a grant acts
-- on the whole API, within the person's role and its scopes. Any app may also trade an access token of its own grant
-- for a narrower one for a single run: shorter, or reading only.

ALTER TABLE oauth_client
    ADD COLUMN kind text NOT NULL DEFAULT 'registered' CONSTRAINT oauth_client_kind_known CHECK (kind IN ('registered', 'partner')),
    ADD COLUMN keycloak_client_id text CONSTRAINT oauth_client_keycloak_client_id_key UNIQUE,
    DROP CONSTRAINT oauth_client_redirects,
    -- An app that registered itself names where codes go back; a partner is registered by an administrator with its
    -- realm client and a secret, allowed at once, and is never sent a code.
    ADD CONSTRAINT oauth_client_kind_shape CHECK (
        (kind = 'registered' AND keycloak_client_id IS NULL AND cardinality(redirect_uris) BETWEEN 1 AND 5)
        OR (kind = 'partner' AND char_length(btrim(keycloak_client_id)) BETWEEN 1 AND 255 AND cardinality(redirect_uris) = 0
            AND status = 'allowed' AND auth_method <> 'none'));

-- A narrower token for one run, read as its grant narrowed, so it never reaches beyond it, and gone with it.
CREATE TABLE oauth_exchanged_token (
    token_hash bytea PRIMARY KEY CONSTRAINT oauth_exchanged_token_is_sha256 CHECK (octet_length(token_hash) = 32),
    grant_id   uuid NOT NULL REFERENCES api_token (id) ON DELETE CASCADE,
    scopes     text[] NOT NULL CONSTRAINT oauth_exchanged_token_known_scopes
               CHECK (scopes <@ ARRAY['read', 'write']::text[] AND 'read' = ANY (scopes)),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oauth_exchanged_token_grant_idx ON oauth_exchanged_token (grant_id);
CREATE INDEX oauth_exchanged_token_expiry_idx ON oauth_exchanged_token (expires_at);

-- An exchanged token ends no later than the grant's access token it was traded for, and holds no scope the grant
-- does not, whoever writes it.
-- +goose StatementBegin
CREATE FUNCTION oauth_exchanged_token_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = public, pg_temp
AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM api_token
                   WHERE id = NEW.grant_id AND client_id IS NOT NULL AND access_expires_at > now()
                     AND NEW.expires_at <= access_expires_at AND NEW.scopes <@ scopes) THEN
        RAISE EXCEPTION 'an exchanged token ends no later than the live grant it narrows, and holds no more'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'oauth_exchanged_token_within_grant';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER oauth_exchanged_token_within_grant BEFORE INSERT OR UPDATE ON oauth_exchanged_token
    FOR EACH ROW EXECUTE FUNCTION oauth_exchanged_token_guard();

ALTER TABLE oauth_exchanged_token ENABLE ROW LEVEL SECURITY;
CREATE POLICY oauth_exchanged_token_admin_role ON oauth_exchanged_token TO bearing_admin USING (true) WITH CHECK (true);
REVOKE ALL ON oauth_exchanged_token FROM bearing_app;

-- Administrators register partner apps as themselves, under the policy that lets them alone see the apps.
CREATE POLICY oauth_client_register_partner ON oauth_client FOR INSERT TO bearing_app
    WITH CHECK ((SELECT current_actor_is_administrator()) AND kind = 'partner');
GRANT INSERT (name, scopes, status, redirect_uris, auth_method, secret_hash, kind, keycloak_client_id) ON oauth_client TO bearing_app;

-- +goose Down
REVOKE INSERT (name, scopes, status, redirect_uris, auth_method, secret_hash, kind, keycloak_client_id) ON oauth_client FROM bearing_app;
DROP POLICY oauth_client_register_partner ON oauth_client;
DROP TABLE oauth_exchanged_token;
DROP FUNCTION oauth_exchanged_token_guard();
DELETE FROM oauth_client WHERE kind = 'partner';
ALTER TABLE oauth_client DROP CONSTRAINT oauth_client_kind_shape,
    DROP COLUMN keycloak_client_id,
    DROP COLUMN kind,
    ADD CONSTRAINT oauth_client_redirects CHECK (cardinality(redirect_uris) BETWEEN 1 AND 5);
