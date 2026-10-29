-- +goose Up
-- Single sign-on: the identities an account signs in with at the provider, with the groups the provider last named;
-- the requests of people the provider vouches for whom nobody has let in yet; and the groups whose members an
-- administrator lets in at once, with a role.

CREATE TABLE account_identity (
    account_id    uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    issuer        text NOT NULL,
    subject       text NOT NULL,
    -- Replaced at every sign-in, so leaving a group at the provider leaves it here.
    groups        text[] NOT NULL DEFAULT '{}',
    last_sign_in  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issuer, subject)
);

CREATE INDEX account_identity_account ON account_identity (account_id);

CREATE TABLE join_request (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    issuer       text NOT NULL,
    subject      text NOT NULL,
    email        citext NOT NULL,
    name         text NOT NULL,
    groups       text[] NOT NULL DEFAULT '{}',
    requested_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);

CREATE TABLE group_admission (
    group_name text PRIMARY KEY CHECK (length(btrim(group_name)) BETWEEN 1 AND 255),
    role       account_role NOT NULL
);

ALTER TABLE account_identity ENABLE ROW LEVEL SECURITY;
ALTER TABLE join_request ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_admission ENABLE ROW LEVEL SECURITY;

CREATE POLICY account_identity_admin_role ON account_identity TO bearing_admin USING (true) WITH CHECK (true);
-- Who signs in through the provider, with which groups, is for everybody signed in to see, as accounts are.
CREATE POLICY account_identity_read ON account_identity FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
-- Letting somebody in who asked to join links their identity to the account made for them.
CREATE POLICY account_identity_admit ON account_identity FOR INSERT TO bearing_app WITH CHECK (current_actor_is_administrator());

CREATE POLICY join_request_admin_role ON join_request TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY join_request_administrators ON join_request TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

CREATE POLICY group_admission_admin_role ON group_admission TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY group_admission_administrators ON group_admission TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

-- +goose Down
DROP TABLE group_admission;
DROP TABLE join_request;
DROP TABLE account_identity;
