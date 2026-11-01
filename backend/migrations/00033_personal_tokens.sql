-- +goose Up
-- Personal access tokens (bearing_pat_), for scripts and assistants that cannot do OAuth: an api_token row of the
-- person's without an app, read only or acting as far as the person may, ending on a day they chose. The person
-- makes their own as bearing_app; nobody makes one for somebody else.

CREATE POLICY api_token_make_own ON api_token FOR INSERT TO bearing_app
    WITH CHECK (account_id = (SELECT current_actor_id()) AND client_id IS NULL AND NOT mcp_only);
GRANT INSERT (account_id, name, scopes, token_hash, expires_at) ON api_token TO bearing_app;

-- +goose Down
REVOKE INSERT (account_id, name, scopes, token_hash, expires_at) ON api_token FROM bearing_app;
DROP POLICY api_token_make_own ON api_token;
