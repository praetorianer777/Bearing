-- +goose Up
-- Roles: what somebody may do is the highest of the role granted to them and the roles granted to the groups the
-- identity provider last named for them, so leaving a group at the provider gives up its role at the next sign-in.
-- Administrators do everything; editors change the organisation and its processes; members read and keep their own
-- profile; readers read.

ALTER TABLE group_admission RENAME TO group_grant;
ALTER INDEX group_admission_pkey RENAME TO group_grant_pkey;
ALTER TABLE group_grant RENAME CONSTRAINT group_admission_group_name_check TO group_grant_group_name_check;
ALTER POLICY group_admission_admin_role ON group_grant RENAME TO group_grant_admin_role;
ALTER POLICY group_admission_administrators ON group_grant RENAME TO group_grant_administrators;

-- The role an account holds, from the role of its own, whether or not it is switched off. Roles order from the most
-- an account may do to the least, so the highest is the least of them. The account's own role is passed in rather
-- than read, so a statement that has just written it, such as an INSERT RETURNING, sees what it wrote. It reads as
-- its owner, so any account's role can be asked for without seeing the identities and grants it comes from.
-- +goose StatementBegin
CREATE FUNCTION account_role_of(who uuid, own account_role) RETURNS account_role
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT min(role) FROM (
        SELECT own AS role
        UNION ALL
        SELECT g.role FROM account_identity i JOIN group_grant g ON g.group_name = ANY (i.groups)
        WHERE i.account_id = who
    ) roles
$$;
-- +goose StatementEnd

-- The role of the person the transaction acts for; NULL for nobody, or for an account switched off.
-- +goose StatementBegin
CREATE FUNCTION current_actor_role() RETURNS account_role
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT account_role_of(id, role) FROM account WHERE id = current_actor_id() AND disabled_at IS NULL
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION current_actor_is_administrator() RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT coalesce(current_actor_role() = 'administrator', false)
$$;
-- +goose StatementEnd

-- Whether the person the transaction acts for may change the organisation and its processes. Every table of the
-- model holds its writes to it.
-- +goose StatementBegin
CREATE FUNCTION current_actor_may_edit() RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT coalesce(current_actor_role() <= 'editor', false)
$$;
-- +goose StatementEnd

-- Members and above keep their own profile.
CREATE POLICY account_update_self ON account FOR UPDATE TO bearing_app
    USING (id = current_actor_id() AND current_actor_role() <= 'member')
    WITH CHECK (id = current_actor_id() AND current_actor_role() <= 'member');

-- A policy sees only the row an update leaves, so whether somebody changed more of their own account than their
-- profile is told here, from the row before. Administrators change any of it; bearing_admin acts for nobody, on the
-- way in.
-- +goose StatementBegin
CREATE FUNCTION account_keep_to_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF current_user = 'bearing_app' AND NOT current_actor_is_administrator()
        AND (NEW.email, NEW.role, NEW.disabled_at, NEW.password_hash) IS DISTINCT FROM
            (OLD.email, OLD.role, OLD.disabled_at, OLD.password_hash) THEN
        RAISE EXCEPTION 'only an administrator changes an account beyond its profile'
            USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER account_keep_to_profile BEFORE UPDATE ON account FOR EACH ROW EXECUTE FUNCTION account_keep_to_profile();

-- +goose Down
DROP TRIGGER account_keep_to_profile ON account;
DROP FUNCTION account_keep_to_profile();
DROP POLICY account_update_self ON account;
DROP FUNCTION current_actor_may_edit();
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION current_actor_is_administrator() RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT EXISTS (
        SELECT 1 FROM account
        WHERE id = current_actor_id() AND role = 'administrator' AND disabled_at IS NULL
    )
$$;
-- +goose StatementEnd
DROP FUNCTION current_actor_role();
DROP FUNCTION account_role_of(uuid, account_role);
ALTER POLICY group_grant_administrators ON group_grant RENAME TO group_admission_administrators;
ALTER POLICY group_grant_admin_role ON group_grant RENAME TO group_admission_admin_role;
ALTER TABLE group_grant RENAME CONSTRAINT group_grant_group_name_check TO group_admission_group_name_check;
ALTER INDEX group_grant_pkey RENAME TO group_admission_pkey;
ALTER TABLE group_grant RENAME TO group_admission;
