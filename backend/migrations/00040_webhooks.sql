-- +goose Up
-- Outbound webhooks (webhooks@1), as Stator and Armature keep them: an endpoint is an address, a secret it was shown
-- once, sealed with BEARING_SECRET_KEY, the topics it takes and a log of every attempt, one row per attempt.
-- Changes to the organisation reach them through webhook_change, which triggers on the model tables write in the
-- transaction that makes the change; the worker turns each into the topics it means, queues a delivery for every
-- endpoint subscribed, and deletes it, in one transaction, so a change is announced once.

CREATE TABLE webhook_endpoint (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    name            text NOT NULL,
    url             text NOT NULL,
    -- Sealed for this endpoint, so it opens on no other row.
    secret_sealed   bytea NOT NULL,
    topics          text[] NOT NULL,
    enabled         boolean NOT NULL DEFAULT true,
    -- The envelope version the endpoint was registered for.
    version         integer NOT NULL DEFAULT 1,
    -- The administrator who last saved it; without one, or once they are no longer an administrator, nothing but a
    -- ping is sent.
    owner_id        uuid REFERENCES account (id) ON DELETE SET NULL,
    -- Failed attempts since the last one delivered, and since when; the worker turns an endpoint off that has
    -- failed long enough.
    failures        integer NOT NULL DEFAULT 0,
    failing_since   timestamptz,
    disabled_reason text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT webhook_endpoint_name_shape CHECK (btrim(name) <> '' AND length(name) <= 100),
    CONSTRAINT webhook_endpoint_url_shape CHECK (url ~ '^https?://[^/?#@\s]+' AND length(url) <= 2000),
    CONSTRAINT webhook_endpoint_topics_known CHECK (cardinality(topics) >= 1 AND topics <@ ARRAY['*', 'position.holder_changed',
        'position.vacated', 'reporting_line.changed', 'unit.changed', 'responsibility.changed']::text[]),
    CONSTRAINT webhook_endpoint_disabled_reason CHECK (disabled_reason IS NULL OR disabled_reason = 'failing'),
    CONSTRAINT webhook_endpoint_failures CHECK (failures >= 0),
    CONSTRAINT webhook_endpoint_version CHECK (version >= 1)
);

CREATE UNIQUE INDEX webhook_endpoint_name_idx ON webhook_endpoint (lower(name));

CREATE TABLE webhook_delivery (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    endpoint_id     uuid NOT NULL REFERENCES webhook_endpoint (id) ON DELETE CASCADE,
    -- The envelope's id, the same on every attempt and redelivery of one event.
    event_id        uuid NOT NULL,
    topic           text NOT NULL,
    -- The envelope's payload, read when the change was announced.
    payload         jsonb NOT NULL,
    occurred_at     timestamptz NOT NULL,
    attempt         integer NOT NULL DEFAULT 1,
    -- A test or a redelivery, which an administrator asked for and which is sent even while the endpoint is off.
    manual          boolean NOT NULL DEFAULT false,
    state           text NOT NULL DEFAULT 'pending',
    status          integer,
    error           text NOT NULL DEFAULT '',
    -- While pending, when it is due; a sender holds it with a lease ahead.
    next_attempt_at timestamptz,
    attempted_at    timestamptz,
    delivered_at    timestamptz,
    version         integer NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT webhook_delivery_attempt_positive CHECK (attempt > 0),
    CONSTRAINT webhook_delivery_state CHECK (state IN ('pending', 'delivered', 'failed', 'withheld', 'cancelled')),
    CONSTRAINT webhook_delivery_due CHECK ((state = 'pending') = (next_attempt_at IS NOT NULL)),
    CONSTRAINT webhook_delivery_payload_object CHECK (jsonb_typeof(payload) = 'object')
);

-- The automatic attempts at one event are numbered once each; sends by hand are numbered apart.
CREATE UNIQUE INDEX webhook_delivery_once ON webhook_delivery (endpoint_id, event_id, attempt) WHERE NOT manual;
CREATE INDEX webhook_delivery_due_idx ON webhook_delivery (next_attempt_at) WHERE state = 'pending';
CREATE INDEX webhook_delivery_endpoint_idx ON webhook_delivery (endpoint_id, created_at DESC, id DESC);
CREATE INDEX webhook_delivery_created_idx ON webhook_delivery (created_at);

-- A change the worker has yet to announce: the row before and after, as the audit log keeps it.
CREATE TABLE webhook_change (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    at         timestamptz NOT NULL DEFAULT now(),
    actor_id   uuid,
    table_name text NOT NULL,
    operation  audit_operation NOT NULL,
    before     jsonb,
    after      jsonb
);

-- +goose StatementBegin
-- Whoever saves an endpoint is its owner, and the database says so rather than the caller. The worker acts for
-- nobody and keeps the owner. Turning an endpoint back on starts its count of failures again.
CREATE FUNCTION webhook_endpoint_stamp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF current_actor_id() IS NOT NULL THEN
        NEW.owner_id := current_actor_id();
        NEW.updated_at := now();
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.enabled AND NOT OLD.enabled THEN
        NEW.failures := 0;
        NEW.failing_since := NULL;
        NEW.disabled_reason := NULL;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER webhook_endpoint_stamp BEFORE INSERT OR UPDATE ON webhook_endpoint
    FOR EACH ROW EXECUTE FUNCTION webhook_endpoint_stamp();

-- +goose StatementBegin
-- Nothing is kept while no endpoint is on, so an installation without webhooks writes nothing more.
CREATE FUNCTION webhook_note() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
DECLARE
    old_row jsonb := CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) - 'created_at' - 'updated_at' - 'search_vector' END;
    new_row jsonb := CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) - 'created_at' - 'updated_at' - 'search_vector' END;
BEGIN
    IF old_row = new_row OR NOT EXISTS (SELECT 1 FROM webhook_endpoint WHERE enabled) THEN
        RETURN NULL;
    END IF;
    INSERT INTO webhook_change (actor_id, table_name, operation, before, after)
    VALUES (current_actor_id(), TG_TABLE_NAME, lower(TG_OP)::audit_operation, old_row, new_row);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON holding FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON reporting_line FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON unit FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON unit_placement FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON position_role FOR EACH ROW EXECUTE FUNCTION webhook_note();
CREATE TRIGGER webhook_note AFTER INSERT OR UPDATE OR DELETE ON deputy FOR EACH ROW EXECUTE FUNCTION webhook_note();
-- A draft saved is nobody's business yet; publishing it is.
CREATE TRIGGER webhook_note AFTER UPDATE ON process_version FOR EACH ROW
    WHEN (OLD.number IS NULL AND NEW.number IS NOT NULL) EXECUTE FUNCTION webhook_note();

ALTER TABLE webhook_endpoint ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_delivery ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_change ENABLE ROW LEVEL SECURITY;

CREATE POLICY webhook_endpoint_admin_role ON webhook_endpoint TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY webhook_delivery_admin_role ON webhook_delivery TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY webhook_change_admin_role ON webhook_change TO bearing_admin USING (true) WITH CHECK (true);
-- Webhooks are the administrators' to see and keep, as in the siblings.
CREATE POLICY webhook_endpoint_administrators ON webhook_endpoint TO bearing_app
    USING ((SELECT current_actor_is_administrator())) WITH CHECK ((SELECT current_actor_is_administrator()));
CREATE POLICY webhook_delivery_administrators ON webhook_delivery FOR SELECT TO bearing_app
    USING ((SELECT current_actor_is_administrator()));

-- The app role never reads a sealed secret back, never sets the owner or the worker's counts, and never writes the
-- log: the worker and a test or redelivery write it as the admin role, which also opens the secret.
REVOKE ALL ON webhook_endpoint, webhook_delivery, webhook_change FROM bearing_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON webhook_endpoint, webhook_delivery, webhook_change TO bearing_admin;
GRANT SELECT (id, name, url, topics, enabled, version, owner_id, failures, failing_since, disabled_reason, created_at, updated_at),
      INSERT (id, name, url, secret_sealed, topics, enabled, version),
      UPDATE (name, url, secret_sealed, topics, enabled, version),
      DELETE
    ON webhook_endpoint TO bearing_app;
GRANT SELECT ON webhook_delivery TO bearing_app;

-- +goose Down
DROP TRIGGER webhook_note ON process_version;
DROP TRIGGER webhook_note ON deputy;
DROP TRIGGER webhook_note ON position_role;
DROP TRIGGER webhook_note ON unit_placement;
DROP TRIGGER webhook_note ON unit;
DROP TRIGGER webhook_note ON reporting_line;
DROP TRIGGER webhook_note ON holding;
DROP FUNCTION webhook_note();
DROP TABLE webhook_change;
DROP TABLE webhook_delivery;
DROP TABLE webhook_endpoint;
DROP FUNCTION webhook_endpoint_stamp();
