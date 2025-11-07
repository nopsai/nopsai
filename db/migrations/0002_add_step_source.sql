ALTER TABLE steps
    ADD COLUMN IF NOT EXISTS source VARCHAR(32);

UPDATE steps
SET source = 'database'
WHERE source IS NULL;

ALTER TABLE steps
    ALTER COLUMN source SET NOT NULL,
    ALTER COLUMN source SET DEFAULT 'database';
