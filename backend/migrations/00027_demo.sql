-- +goose Up
-- When the demo organisation was loaded, so loading it again changes nothing.
ALTER TABLE installation ADD COLUMN demo_loaded_at timestamptz;

-- +goose Down
ALTER TABLE installation DROP COLUMN demo_loaded_at;
