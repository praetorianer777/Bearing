-- +goose Up
-- Bearing beside Armature (#92), as Stator connects to it: an administrator, or Dynamo's connect job, names the
-- Armature and the partner app Bearing is registered there as; each person's realm token is traded for a grant of
-- theirs when they sign in, and Bearing reads Armature as them with it. Secrets and tokens are sealed with
-- BEARING_SECRET_KEY; the app role never reads them back.

CREATE TABLE armature_connection (
    one           boolean PRIMARY KEY DEFAULT true CHECK (one),
    base_url      text NOT NULL CONSTRAINT armature_connection_url CHECK (base_url ~ '^https?://[^/?#@\s]+' AND length(base_url) <= 2000),
    client_id     text NOT NULL CONSTRAINT armature_connection_client CHECK (btrim(client_id) <> '' AND length(client_id) <= 200),
    secret_sealed bytea NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- A person's grant at Armature: its access token until access_expires_at, and the refresh token that renews it.
CREATE TABLE armature_grant (
    account_id        uuid PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,
    access_sealed     bytea NOT NULL,
    refresh_sealed    bytea NOT NULL,
    access_expires_at timestamptz NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE armature_connection ENABLE ROW LEVEL SECURITY;
ALTER TABLE armature_grant ENABLE ROW LEVEL SECURITY;
CREATE POLICY armature_connection_admin_role ON armature_connection TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY armature_grant_admin_role ON armature_grant TO bearing_admin USING (true) WITH CHECK (true);
-- Administrators keep the connection; everybody signed in may see that there is one, to be told whether Armature is
-- connected. A person sees and ends their own grant.
CREATE POLICY armature_connection_read ON armature_connection FOR SELECT TO bearing_app USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY armature_connection_administrators ON armature_connection TO bearing_app
    USING ((SELECT current_actor_is_administrator())) WITH CHECK ((SELECT current_actor_is_administrator()));
CREATE POLICY armature_grant_own ON armature_grant FOR SELECT TO bearing_app USING (account_id = (SELECT current_actor_id()));
CREATE POLICY armature_grant_end ON armature_grant FOR DELETE TO bearing_app USING (account_id = (SELECT current_actor_id()));

REVOKE ALL ON armature_connection, armature_grant FROM bearing_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON armature_connection, armature_grant TO bearing_admin;
GRANT SELECT (one, base_url, client_id, updated_at), INSERT, UPDATE, DELETE ON armature_connection TO bearing_app;
GRANT SELECT (account_id, access_expires_at, created_at), DELETE ON armature_grant TO bearing_app;

-- +goose Down
DROP TABLE armature_grant;
DROP TABLE armature_connection;
