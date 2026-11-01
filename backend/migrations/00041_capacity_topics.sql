-- +goose Up
-- The topics of resource planning (#94): a holding planned, started or ended, a person leaving, and a gap in the
-- organisation's capacity opening, closing or changing. A holding starts and ends on its day, which nobody writes, so
-- the worker sweeps each day once, as webhook_sweep records; gaps are compared with the ones the worker saw last,
-- which webhook_gap keeps over the window webhook_gap_window names.

ALTER TABLE webhook_endpoint DROP CONSTRAINT webhook_endpoint_topics_known;
ALTER TABLE webhook_endpoint ADD CONSTRAINT webhook_endpoint_topics_known CHECK (cardinality(topics) >= 1 AND topics <@ ARRAY['*',
    'position.holder_changed', 'position.vacated', 'reporting_line.changed', 'unit.changed', 'responsibility.changed',
    'holding.planned', 'holding.started', 'holding.ended', 'person.left', 'capacity.gap_changed']::text[]);

-- A person leaving is announced; a position's seats and a target change the gaps.
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON person FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON position FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON target FOR EACH ROW EXECUTE FUNCTION webhook_note();

CREATE TABLE webhook_sweep (
    day date PRIMARY KEY
);

CREATE TABLE webhook_gap (
    kind         text NOT NULL,
    subject_kind text NOT NULL,
    subject_id   uuid NOT NULL,
    from_day     date NOT NULL,
    to_day       date NOT NULL,
    size         double precision NOT NULL,
    name         text NOT NULL,
    unit_id      uuid,
    PRIMARY KEY (kind, subject_kind, subject_id, from_day)
);

-- The one window the gaps were read over; none until the worker has looked once.
CREATE TABLE webhook_gap_window (
    one      boolean PRIMARY KEY DEFAULT true CHECK (one),
    from_day date NOT NULL,
    to_day   date NOT NULL
);

ALTER TABLE webhook_sweep ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_gap ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_gap_window ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_sweep_admin_role ON webhook_sweep TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY webhook_gap_admin_role ON webhook_gap TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY webhook_gap_window_admin_role ON webhook_gap_window TO bearing_admin USING (true) WITH CHECK (true);
REVOKE ALL ON webhook_sweep, webhook_gap, webhook_gap_window FROM bearing_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON webhook_sweep, webhook_gap, webhook_gap_window TO bearing_admin;

-- +goose Down
DROP TABLE webhook_gap_window;
DROP TABLE webhook_gap;
DROP TABLE webhook_sweep;
DROP TRIGGER webhook_note ON target;
DROP TRIGGER webhook_note ON position;
DROP TRIGGER webhook_note ON person;
ALTER TABLE webhook_endpoint DROP CONSTRAINT webhook_endpoint_topics_known;
ALTER TABLE webhook_endpoint ADD CONSTRAINT webhook_endpoint_topics_known CHECK (cardinality(topics) >= 1 AND topics <@ ARRAY['*',
    'position.holder_changed', 'position.vacated', 'reporting_line.changed', 'unit.changed', 'responsibility.changed']::text[]);
