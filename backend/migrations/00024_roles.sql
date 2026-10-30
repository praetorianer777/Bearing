-- +goose Up
-- Roles and deputies. A role is a function that does not follow the tree, such as "Invoice approver"; positions
-- carry it for a time, so whoever holds them has it, and people may hold it directly as well. A position's deputies
-- are other positions, in the order they stand in. Editors keep both.

CREATE TABLE role (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    -- What other tools name the role by; it does not change with the name.
    key         text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER role_touch BEFORE UPDATE ON role FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TABLE position_role (
    position_id uuid NOT NULL REFERENCES position (id) ON DELETE CASCADE,
    role_id     uuid NOT NULL REFERENCES role (id),
    valid_from  date NOT NULL,
    -- The first day it no longer holds; NULL is open.
    valid_to    date CHECK (valid_to > valid_from),
    CONSTRAINT position_role_once EXCLUDE USING gist (position_id WITH =, role_id WITH =, daterange(valid_from, valid_to) WITH &&)
);

CREATE INDEX position_role_role ON position_role (role_id);

-- A holding is of a position or, held directly, of a role.
ALTER TABLE holding ALTER COLUMN position_id DROP NOT NULL;
ALTER TABLE holding ADD COLUMN role_id uuid REFERENCES role (id);
ALTER TABLE holding ADD CONSTRAINT holding_one_thing CHECK ((position_id IS NULL) <> (role_id IS NULL));
ALTER TABLE holding ADD CONSTRAINT holding_role_once
    EXCLUDE USING gist (person_id WITH =, role_id WITH =, daterange(valid_from, valid_to) WITH &&);
CREATE INDEX holding_role ON holding (role_id, valid_from);

-- Roles have no seats; the check is for positions alone.
DROP TRIGGER holding_within_seats ON holding;
CREATE TRIGGER holding_within_seats AFTER INSERT OR UPDATE ON holding FOR EACH ROW WHEN (NEW.position_id IS NOT NULL)
    EXECUTE FUNCTION holding_within_seats();

CREATE TABLE deputy (
    position_id uuid NOT NULL REFERENCES position (id) ON DELETE CASCADE,
    deputy_id   uuid NOT NULL REFERENCES position (id) ON DELETE CASCADE,
    -- First stands in first.
    rank        smallint NOT NULL CHECK (rank >= 1),
    PRIMARY KEY (position_id, deputy_id),
    UNIQUE (position_id, rank),
    CHECK (deputy_id <> position_id)
);

CREATE INDEX deputy_deputy ON deputy (deputy_id);

ALTER TABLE role ENABLE ROW LEVEL SECURITY;
ALTER TABLE position_role ENABLE ROW LEVEL SECURITY;
ALTER TABLE deputy ENABLE ROW LEVEL SECURITY;

CREATE POLICY role_admin_role ON role TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY role_read ON role FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY role_editors ON role TO bearing_app USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

CREATE POLICY position_role_admin_role ON position_role TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY position_role_read ON position_role FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY position_role_editors ON position_role TO bearing_app
    USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

CREATE POLICY deputy_admin_role ON deputy TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY deputy_read ON deputy FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY deputy_editors ON deputy TO bearing_app USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

-- +goose Down
DROP TABLE deputy;
DROP TRIGGER holding_within_seats ON holding;
CREATE TRIGGER holding_within_seats AFTER INSERT OR UPDATE ON holding FOR EACH ROW EXECUTE FUNCTION holding_within_seats();
DELETE FROM holding WHERE role_id IS NOT NULL;
ALTER TABLE holding DROP CONSTRAINT holding_role_once;
ALTER TABLE holding DROP CONSTRAINT holding_one_thing;
ALTER TABLE holding DROP COLUMN role_id;
ALTER TABLE holding ALTER COLUMN position_id SET NOT NULL;
DROP TABLE position_role;
DROP TABLE role;
