-- +goose Up
-- Units: the tree of the organisation, its divisions, departments and teams, each of a type administrators define.
-- Where a unit sits is dated: a move from a day ends the old placement there and starts the new one, so the tree can
-- be read as it was or as it will be. Editors change units; administrators change their types.

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE unit_type (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    -- One of the web client's icon names, so a type draws without a file.
    icon       text NOT NULL DEFAULT 'unit' CHECK (icon ~ '^[a-z][a-z-]{0,39}$'),
    colour     text NOT NULL DEFAULT '#6f685d' CHECK (colour ~ '^#[0-9a-f]{6}$'),
    -- Where the type sorts in lists, larger units first.
    rank       integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX unit_type_name ON unit_type (lower(name));

CREATE TABLE unit (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    type_id     uuid NOT NULL REFERENCES unit_type (id),
    name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    short_name  text CHECK (length(btrim(short_name)) BETWEEN 1 AND 20),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    -- From this day on the unit is gone; its placements and history stay for the days before.
    archived_on date,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX unit_type_idx ON unit (type_id);

CREATE TABLE unit_placement (
    unit_id    uuid NOT NULL REFERENCES unit (id) ON DELETE CASCADE,
    -- NULL places the unit at the root, which one unit at a time may be.
    parent_id  uuid REFERENCES unit (id),
    valid_from date NOT NULL,
    -- The first day it no longer holds; NULL is open.
    valid_to   date CHECK (valid_to > valid_from),
    -- A unit sits in one place on any day.
    CONSTRAINT unit_placement_one_place EXCLUDE USING gist (unit_id WITH =, daterange(valid_from, valid_to) WITH &&),
    -- The organisation has one root on any day.
    CONSTRAINT unit_placement_one_root EXCLUDE USING gist (daterange(valid_from, valid_to) WITH &&) WHERE (parent_id IS NULL),
    CHECK (parent_id IS DISTINCT FROM unit_id)
);

CREATE INDEX unit_placement_parent ON unit_placement (parent_id);

-- A placement must not make a unit its own ancestor. Every placement overlapping the new one counts, which refuses
-- a few moves that would only cycle through a day the new placement does not cover, and keeps the check simple.
-- Moves take a lock first, so two moves at once cannot each pass and together make a cycle.
-- +goose StatementBegin
CREATE FUNCTION unit_placement_no_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtext('unit_placement'));
    IF EXISTS (
        WITH RECURSIVE up (id) AS (
            SELECT NEW.parent_id
            UNION
            SELECT p.parent_id FROM unit_placement p JOIN up ON p.unit_id = up.id
            WHERE p.parent_id IS NOT NULL
              AND daterange(p.valid_from, p.valid_to) && daterange(NEW.valid_from, NEW.valid_to)
        )
        SELECT 1 FROM up WHERE id = NEW.unit_id
    ) THEN
        RAISE EXCEPTION 'that move would put the unit inside itself' USING ERRCODE = 'integrity_constraint_violation',
            CONSTRAINT = 'unit_placement_no_cycle';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER unit_placement_no_cycle BEFORE INSERT OR UPDATE ON unit_placement
    FOR EACH ROW EXECUTE FUNCTION unit_placement_no_cycle();

ALTER TABLE unit_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE unit ENABLE ROW LEVEL SECURITY;
ALTER TABLE unit_placement ENABLE ROW LEVEL SECURITY;

CREATE POLICY unit_type_admin_role ON unit_type TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY unit_type_read ON unit_type FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY unit_type_administrators ON unit_type TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

CREATE POLICY unit_admin_role ON unit TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY unit_read ON unit FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY unit_editors ON unit TO bearing_app USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

CREATE POLICY unit_placement_admin_role ON unit_placement TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY unit_placement_read ON unit_placement FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY unit_placement_editors ON unit_placement TO bearing_app
    USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

CREATE TRIGGER unit_touch BEFORE UPDATE ON unit FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE unit_placement;
DROP FUNCTION unit_placement_no_cycle();
DROP TABLE unit;
DROP TABLE unit_type;
