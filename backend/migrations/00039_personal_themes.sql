-- +goose Up
-- Themes become their makers', as in Exciter and Stator: everybody sees their own and the shared ones, only the owner
-- changes a theme, an administrator may tidy a shared one, and each person's choice is kept here so it follows them
-- between devices. The installation's themes so far were everybody's, so they stay shared, owned by whoever imported
-- them or else the first administrator.

ALTER TABLE theme ADD COLUMN owner_id uuid REFERENCES account (id) ON DELETE CASCADE;
ALTER TABLE theme ADD COLUMN shared boolean NOT NULL DEFAULT false;
UPDATE theme SET shared = true, owner_id = coalesce(created_by, (
    SELECT id FROM account WHERE role = 'administrator' ORDER BY created_at, id LIMIT 1));
DELETE FROM theme WHERE owner_id IS NULL;
ALTER TABLE theme ALTER COLUMN owner_id SET NOT NULL;
ALTER TABLE theme DROP COLUMN created_by;

-- Names are unique per owner, so two people may each call theirs "Night".
DROP INDEX theme_name;
CREATE UNIQUE INDEX theme_owner_name ON theme (owner_id, lower(name));
CREATE INDEX theme_shared ON theme (shared) WHERE shared;

-- The default is shown to everybody, so it has to be shared; a theme taken private stops being it. Only an
-- administrator names it, and a theme keeps the person who made it.
-- +goose StatementBegin
CREATE FUNCTION theme_guard() RETURNS trigger
    LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.owner_id IS DISTINCT FROM OLD.owner_id THEN
        RAISE EXCEPTION 'a theme keeps the person who made it' USING ERRCODE = 'check_violation';
    END IF;
    IF NOT NEW.shared THEN
        NEW.is_default := false;
    END IF;
    IF NEW.is_default AND NOT OLD.is_default AND current_user <> 'bearing_admin' AND NOT current_actor_is_administrator() THEN
        RAISE EXCEPTION 'only an administrator names the theme everybody sees' USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER theme_guard BEFORE UPDATE ON theme FOR EACH ROW EXECUTE FUNCTION theme_guard();
ALTER TABLE theme ADD CONSTRAINT theme_default_shared CHECK (NOT is_default OR shared);

-- Who chose what, one row a person. A row with no theme is a choice too: the built-in look over the default.
-- Deleting a theme deletes the rows that chose it, so its people return to the default.
CREATE TABLE user_theme (
    account_id uuid PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,
    theme_id   uuid REFERENCES theme (id) ON DELETE CASCADE
);

CREATE INDEX user_theme_theme ON user_theme (theme_id);

DROP POLICY theme_read ON theme;
DROP POLICY theme_administrators ON theme;
CREATE POLICY theme_visible ON theme FOR SELECT TO bearing_app
    USING (owner_id = current_actor_id() OR shared);
CREATE POLICY theme_made_as_oneself ON theme FOR INSERT TO bearing_app
    WITH CHECK (owner_id = current_actor_id());
CREATE POLICY theme_changed_by_owner ON theme FOR UPDATE TO bearing_app
    USING (owner_id = current_actor_id() OR (shared AND current_actor_is_administrator()))
    WITH CHECK (owner_id = current_actor_id() OR current_actor_is_administrator());
CREATE POLICY theme_deleted_by_owner ON theme FOR DELETE TO bearing_app
    USING (owner_id = current_actor_id() OR (shared AND current_actor_is_administrator()));

-- A file follows its theme: seen with it, added or removed by whoever may change it. The subqueries on theme pass
-- through its own policies above.
DROP POLICY theme_asset_read ON theme_asset;
DROP POLICY theme_asset_administrators ON theme_asset;
CREATE POLICY theme_asset_visible ON theme_asset FOR SELECT TO bearing_app
    USING (EXISTS (SELECT 1 FROM theme t WHERE t.id = theme_asset.theme_id));
CREATE POLICY theme_asset_added_by_owner ON theme_asset FOR INSERT TO bearing_app
    WITH CHECK (EXISTS (SELECT 1 FROM theme t WHERE t.id = theme_asset.theme_id
                        AND (t.owner_id = current_actor_id() OR (t.shared AND current_actor_is_administrator()))));
CREATE POLICY theme_asset_removed_by_owner ON theme_asset FOR DELETE TO bearing_app
    USING (EXISTS (SELECT 1 FROM theme t WHERE t.id = theme_asset.theme_id
                   AND (t.owner_id = current_actor_id() OR (t.shared AND current_actor_is_administrator()))));

ALTER TABLE user_theme ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_theme_admin_role ON user_theme TO bearing_admin USING (true) WITH CHECK (true);
-- A choice is the chooser's own, and of a theme they may see: the foreign key alone would take anybody's.
CREATE POLICY user_theme_own_choice ON user_theme TO bearing_app
    USING (account_id = current_actor_id())
    WITH CHECK (account_id = current_actor_id()
                AND (theme_id IS NULL OR EXISTS (SELECT 1 FROM theme t WHERE t.id = user_theme.theme_id)));

-- How many people chose a theme the caller may see. SECURITY DEFINER, since each person sees only their own choice;
-- it answers 0 for a theme out of the caller's sight.
-- +goose StatementBegin
CREATE FUNCTION theme_in_use(theme uuid) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp
AS $$
    SELECT count(*)::integer FROM user_theme ut
    JOIN theme t ON t.id = ut.theme_id
    WHERE ut.theme_id = theme_in_use.theme AND (t.owner_id = current_actor_id() OR t.shared)
$$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON user_theme TO bearing_app, bearing_admin;
GRANT EXECUTE ON FUNCTION theme_in_use(uuid) TO bearing_app;

-- +goose Down
DROP FUNCTION theme_in_use(uuid);
DROP TABLE user_theme;
DROP POLICY theme_asset_visible ON theme_asset;
DROP POLICY theme_asset_added_by_owner ON theme_asset;
DROP POLICY theme_asset_removed_by_owner ON theme_asset;
CREATE POLICY theme_asset_read ON theme_asset FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY theme_asset_administrators ON theme_asset TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());
DROP POLICY theme_visible ON theme;
DROP POLICY theme_made_as_oneself ON theme;
DROP POLICY theme_changed_by_owner ON theme;
DROP POLICY theme_deleted_by_owner ON theme;
CREATE POLICY theme_read ON theme FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY theme_administrators ON theme TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());
ALTER TABLE theme DROP CONSTRAINT theme_default_shared;
DROP TRIGGER theme_guard ON theme;
DROP FUNCTION theme_guard();
DROP INDEX theme_shared;
DROP INDEX theme_owner_name;
DELETE FROM theme t WHERE EXISTS (SELECT 1 FROM theme o WHERE lower(o.name) = lower(t.name) AND o.id < t.id);
CREATE UNIQUE INDEX theme_name ON theme (lower(name));
ALTER TABLE theme ADD COLUMN created_by uuid REFERENCES account (id) ON DELETE SET NULL;
UPDATE theme SET created_by = owner_id;
ALTER TABLE theme DROP COLUMN shared;
ALTER TABLE theme DROP COLUMN owner_id;
