ALTER TABLE triggers
    ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'database';

-- Ensure existing rows get the default value explicitly
UPDATE triggers SET source = 'database' WHERE source IS NULL;
