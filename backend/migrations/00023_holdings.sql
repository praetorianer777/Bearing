-- +goose Up
-- Holdings: a person holds a position from a day, until a day or for good, at a share of their week. A position
-- nobody holds on a day is vacant then; it has as many seats as people may hold it at once, one unless it says
-- otherwise. Editors keep holdings.

ALTER TABLE position ADD COLUMN seats smallint NOT NULL DEFAULT 1 CHECK (seats BETWEEN 1 AND 1000);

CREATE TABLE holding (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    person_id   uuid NOT NULL REFERENCES person (id) ON DELETE CASCADE,
    position_id uuid NOT NULL REFERENCES position (id) ON DELETE CASCADE,
    -- The share of the person's week the holding takes, in percent.
    share       smallint NOT NULL DEFAULT 100 CHECK (share BETWEEN 1 AND 100),
    valid_from  date NOT NULL,
    -- The first day it no longer holds; NULL is open.
    valid_to    date CHECK (valid_to > valid_from),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT holding_once EXCLUDE USING gist (person_id WITH =, position_id WITH =, daterange(valid_from, valid_to) WITH &&)
);

CREATE INDEX holding_position ON holding (position_id, valid_from);
CREATE INDEX holding_person ON holding (person_id, valid_from);

-- Whether more people hold a position than it has seats on some day of a span. The count only rises where a
-- holding starts, so the days to look at are the starts within the span and the span's first day.
-- +goose StatementBegin
CREATE FUNCTION position_overfilled(pos uuid, seats int, during daterange) RETURNS boolean
    LANGUAGE sql STABLE SET search_path = public
AS $$
    SELECT EXISTS (
        SELECT 1 FROM holding h
        WHERE h.position_id = pos AND daterange(h.valid_from, h.valid_to) && during
          AND (SELECT count(*) FROM holding o WHERE o.position_id = pos
                 AND daterange(o.valid_from, o.valid_to) @> greatest(h.valid_from, lower(during))) > seats
    )
$$;
-- +goose StatementEnd

-- The check runs after the row is written, so it counts it, and takes a lock per position first so two holdings
-- written at once cannot together take more seats than there are.
-- +goose StatementBegin
CREATE FUNCTION holding_within_seats() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    seats int;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('holding'), hashtext(NEW.position_id::text));
    SELECT p.seats INTO seats FROM position p WHERE p.id = NEW.position_id;
    IF position_overfilled(NEW.position_id, seats, daterange(NEW.valid_from, NEW.valid_to)) THEN
        RAISE EXCEPTION 'more people would hold the position than it has seats' USING ERRCODE = 'integrity_constraint_violation',
            CONSTRAINT = 'holding_within_seats';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER holding_within_seats AFTER INSERT OR UPDATE ON holding FOR EACH ROW EXECUTE FUNCTION holding_within_seats();
CREATE TRIGGER holding_touch BEFORE UPDATE ON holding FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- Fewer seats must still fit the holdings from today on; the past keeps the seats it had.
-- +goose StatementBegin
CREATE FUNCTION position_keep_seats() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('holding'), hashtext(NEW.id::text));
    IF position_overfilled(NEW.id, NEW.seats, daterange(current_date, NULL)) THEN
        RAISE EXCEPTION 'more people hold the position than it would have seats' USING ERRCODE = 'integrity_constraint_violation',
            CONSTRAINT = 'holding_within_seats';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER position_keep_seats BEFORE UPDATE OF seats ON position FOR EACH ROW WHEN (NEW.seats < OLD.seats)
    EXECUTE FUNCTION position_keep_seats();

ALTER TABLE holding ENABLE ROW LEVEL SECURITY;

CREATE POLICY holding_admin_role ON holding TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY holding_read ON holding FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY holding_editors ON holding TO bearing_app USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());

-- +goose Down
DROP TRIGGER position_keep_seats ON position;
DROP FUNCTION position_keep_seats();
DROP TABLE holding;
DROP FUNCTION holding_within_seats();
DROP FUNCTION position_overfilled(uuid, int, daterange);
ALTER TABLE position DROP COLUMN seats;
