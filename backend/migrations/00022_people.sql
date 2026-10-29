-- +goose Up
-- People: everybody in the organisation, whether or not they sign in. A person may be linked to an account here, be
-- known to the realm by issuer and sub, or be neither, as somebody external is; a sign-in links the account to its
-- person. People who leave keep their history, marked as gone from a day. Editors keep the people; somebody signed
-- in keeps those fields of their own entry that administrators allow.

CREATE TABLE person (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    email      citext CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+$' AND length(email) <= 254),
    phone      text CHECK (length(phone) <= 50),
    location   text CHECK (length(location) <= 200),
    account_id uuid UNIQUE REFERENCES account (id) ON DELETE SET NULL,
    issuer     text,
    subject    text,
    -- From this day on the person has left; the days before keep them.
    left_on    date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((issuer IS NULL) = (subject IS NULL)),
    UNIQUE (issuer, subject)
);

-- An address names one person, so a sign-in finds one entry to link to.
CREATE UNIQUE INDEX person_email ON person (email) WHERE email IS NOT NULL;
CREATE INDEX person_name ON person (lower(name));

CREATE TABLE person_photo (
    person_id    uuid PRIMARY KEY REFERENCES person (id) ON DELETE CASCADE,
    -- What the bytes are, read from them on the way in.
    content_type text NOT NULL,
    data         bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 1048576),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- The fields of their own entry that somebody signed in may change.
ALTER TABLE installation ADD COLUMN self_editable text[] NOT NULL DEFAULT '{phone,location,photo}'
    CHECK (self_editable <@ ARRAY['name', 'email', 'phone', 'location', 'photo']);

-- Whether the person the transaction acts for may change this field of their own entry.
-- +goose StatementBegin
CREATE FUNCTION current_actor_may_edit_own(field text) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    SELECT coalesce(current_actor_role() <= 'member', false)
        AND EXISTS (SELECT 1 FROM installation WHERE field = ANY (self_editable))
$$;
-- +goose StatementEnd

-- A policy sees only the row an update leaves, so which fields changed is told here, from the row before.
-- +goose StatementBegin
CREATE FUNCTION person_keep_to_allowed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF current_user <> 'bearing_app' OR current_actor_may_edit() THEN
        RETURN NEW;
    END IF;
    IF (NEW.account_id, NEW.issuer, NEW.subject, NEW.left_on) IS DISTINCT FROM (OLD.account_id, OLD.issuer, OLD.subject, OLD.left_on)
        OR NEW.name IS DISTINCT FROM OLD.name AND NOT current_actor_may_edit_own('name')
        OR NEW.email IS DISTINCT FROM OLD.email AND NOT current_actor_may_edit_own('email')
        OR NEW.phone IS DISTINCT FROM OLD.phone AND NOT current_actor_may_edit_own('phone')
        OR NEW.location IS DISTINCT FROM OLD.location AND NOT current_actor_may_edit_own('location') THEN
        RAISE EXCEPTION 'that field of your entry is kept by editors' USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER person_keep_to_allowed BEFORE UPDATE ON person FOR EACH ROW EXECUTE FUNCTION person_keep_to_allowed();
CREATE TRIGGER person_touch BEFORE UPDATE ON person FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

ALTER TABLE person ENABLE ROW LEVEL SECURITY;
ALTER TABLE person_photo ENABLE ROW LEVEL SECURITY;

CREATE POLICY person_admin_role ON person TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY person_read ON person FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY person_editors ON person TO bearing_app USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
CREATE POLICY person_update_own ON person FOR UPDATE TO bearing_app
    USING (account_id = current_actor_id() AND current_actor_role() <= 'member')
    WITH CHECK (account_id = current_actor_id() AND current_actor_role() <= 'member');

CREATE POLICY person_photo_admin_role ON person_photo TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY person_photo_read ON person_photo FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY person_photo_editors ON person_photo TO bearing_app
    USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
CREATE POLICY person_photo_own ON person_photo TO bearing_app
    USING (current_actor_may_edit_own('photo') AND person_id IN (SELECT id FROM person WHERE account_id = current_actor_id()))
    WITH CHECK (current_actor_may_edit_own('photo') AND person_id IN (SELECT id FROM person WHERE account_id = current_actor_id()));

-- +goose Down
DROP TABLE person_photo;
DROP TABLE person;
DROP FUNCTION person_keep_to_allowed();
DROP FUNCTION current_actor_may_edit_own(text);
ALTER TABLE installation DROP COLUMN self_editable;
