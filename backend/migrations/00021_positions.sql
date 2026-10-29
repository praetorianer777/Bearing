-- +goose Up
-- Positions and reporting lines. A position is a slot in a unit, such as "Head of Quality", which people hold; it
-- reports to another position, not to a person, so vacancies and people moving leave the graph standing. A line is
-- solid (disciplinary) or dotted (functional) and dated like every relation. Editors change both.

CREATE TABLE position (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    -- A position stays in its unit; a reorganisation that moves the work makes a new position there.
    unit_id      uuid NOT NULL REFERENCES unit (id),
    title        text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    description  text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    head_of_unit boolean NOT NULL DEFAULT false,
    exists_from  date NOT NULL,
    -- From this day on the position is gone; the days before keep it.
    archived_on  date CHECK (archived_on > exists_from),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX position_unit ON position (unit_id);
-- A unit has one head at a time.
CREATE UNIQUE INDEX position_one_head ON position (unit_id) WHERE head_of_unit AND archived_on IS NULL;

CREATE TYPE line_kind AS ENUM ('solid', 'dotted');

CREATE TABLE reporting_line (
    position_id   uuid NOT NULL REFERENCES position (id) ON DELETE CASCADE,
    reports_to_id uuid NOT NULL REFERENCES position (id),
    kind          line_kind NOT NULL,
    valid_from    date NOT NULL,
    -- The first day it no longer holds; NULL is open.
    valid_to      date CHECK (valid_to > valid_from),
    CHECK (reports_to_id <> position_id),
    -- One solid line per position on any day; dotted lines as many as the matrix needs, but not twice to the same.
    -- A dotted line and a solid one to the same position may overlap, so a dotted line can turn solid on a day.
    CONSTRAINT reporting_line_one_solid EXCLUDE USING gist (position_id WITH =, daterange(valid_from, valid_to) WITH &&)
        WHERE (kind = 'solid'),
    CONSTRAINT reporting_line_once EXCLUDE USING gist
        (position_id WITH =, reports_to_id WITH =, kind WITH =, daterange(valid_from, valid_to) WITH &&)
);

CREATE INDEX reporting_line_position ON reporting_line (position_id, valid_from);
CREATE INDEX reporting_line_reports_to ON reporting_line (reports_to_id);

-- Solid lines must not run in a circle, on any day the new line covers; dotted lines may cross the tree as they
-- like. As with units, the check takes a lock so two changes at once cannot together close a circle.
-- +goose StatementBegin
CREATE FUNCTION reporting_line_no_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.kind <> 'solid' THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtext('reporting_line'));
    IF EXISTS (
        WITH RECURSIVE up (id) AS (
            SELECT NEW.reports_to_id
            UNION
            SELECT l.reports_to_id FROM reporting_line l JOIN up ON l.position_id = up.id
            WHERE l.kind = 'solid'
              AND daterange(l.valid_from, l.valid_to) && daterange(NEW.valid_from, NEW.valid_to)
        )
        SELECT 1 FROM up WHERE id = NEW.position_id
    ) THEN
        RAISE EXCEPTION 'that line would have the position report to itself' USING ERRCODE = 'integrity_constraint_violation',
            CONSTRAINT = 'reporting_line_no_cycle';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER reporting_line_no_cycle BEFORE INSERT OR UPDATE ON reporting_line
    FOR EACH ROW EXECUTE FUNCTION reporting_line_no_cycle();

ALTER TABLE position ENABLE ROW LEVEL SECURITY;
ALTER TABLE reporting_line ENABLE ROW LEVEL SECURITY;

CREATE POLICY position_admin_role ON position TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY position_read ON position FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY position_editors ON position TO bearing_app USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

CREATE POLICY reporting_line_admin_role ON reporting_line TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY reporting_line_read ON reporting_line FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY reporting_line_editors ON reporting_line TO bearing_app
    USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

CREATE TRIGGER position_touch BEFORE UPDATE ON position FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE reporting_line;
DROP FUNCTION reporting_line_no_cycle();
DROP TYPE line_kind;
DROP TABLE position;
