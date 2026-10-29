-- +goose Up
-- Themes: the installation's redefinitions of the tokens and glyphs, in the suite's armature-theme/1 shape, with the
-- files they draw from. Administrators import, export and remove them and name the one everybody starts from;
-- everybody signed in reads them, to choose one.

CREATE TABLE theme (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    spec       jsonb NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    created_by uuid REFERENCES account (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX theme_name ON theme (lower(name));
-- At most one theme is the installation's; none leaves everybody on the built-in look.
CREATE UNIQUE INDEX theme_one_default ON theme (is_default) WHERE is_default;

CREATE TABLE theme_asset (
    theme_id     uuid NOT NULL REFERENCES theme (id) ON DELETE CASCADE,
    id           uuid NOT NULL DEFAULT uuidv7(),
    name         text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    -- What the bytes are, read from them on the way in; never what the file claimed.
    content_type text NOT NULL,
    data         bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 4194304),
    PRIMARY KEY (theme_id, id)
);

ALTER TABLE theme ENABLE ROW LEVEL SECURITY;
ALTER TABLE theme_asset ENABLE ROW LEVEL SECURITY;

CREATE POLICY theme_admin_role ON theme TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY theme_read ON theme FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY theme_administrators ON theme TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

CREATE POLICY theme_asset_admin_role ON theme_asset TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY theme_asset_read ON theme_asset FOR SELECT TO bearing_app USING (current_actor_id() IS NOT NULL);
CREATE POLICY theme_asset_administrators ON theme_asset TO bearing_app
    USING (current_actor_is_administrator()) WITH CHECK (current_actor_is_administrator());

CREATE TRIGGER theme_touch BEFORE UPDATE ON theme FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE theme_asset;
DROP TABLE theme;
