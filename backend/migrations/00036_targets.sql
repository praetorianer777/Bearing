-- +goose Up
-- Targets: what a unit, a position or a role should have, in full-time equivalents, from a day until a day or for
-- good. A subject has one target on any day; setting one for a span replaces what was set for those days. Editors
-- keep targets, and the log keeps each change.

CREATE TABLE target (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    unit_id     uuid REFERENCES unit (id) ON DELETE CASCADE,
    position_id uuid REFERENCES position (id) ON DELETE CASCADE,
    role_id     uuid REFERENCES role (id) ON DELETE CASCADE,
    fte         numeric(8, 2) NOT NULL CHECK (fte >= 0 AND fte <= 100000),
    valid_from  date NOT NULL,
    -- The first day it no longer holds; NULL is open.
    valid_to    date CHECK (valid_to > valid_from),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT target_one_subject CHECK (num_nonnulls(unit_id, position_id, role_id) = 1),
    CONSTRAINT target_once EXCLUDE USING gist
        (coalesce(unit_id, position_id, role_id) WITH =, daterange(valid_from, valid_to) WITH &&)
);

CREATE INDEX target_unit ON target (unit_id, valid_from) WHERE unit_id IS NOT NULL;
CREATE INDEX target_position ON target (position_id, valid_from) WHERE position_id IS NOT NULL;
CREATE INDEX target_role ON target (role_id, valid_from) WHERE role_id IS NOT NULL;

CREATE TRIGGER target_touch BEFORE UPDATE ON target FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON target FOR EACH ROW EXECUTE FUNCTION audit_change();

ALTER TABLE target ENABLE ROW LEVEL SECURITY;

CREATE POLICY target_admin_role ON target TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY target_read ON target FOR SELECT TO bearing_app USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY target_editors ON target TO bearing_app
    USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

-- +goose Down
DROP TABLE target;
