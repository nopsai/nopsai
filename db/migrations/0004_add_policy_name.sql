ALTER TABLE role_permissions
ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';

UPDATE role_permissions
SET name = (
    CASE
        WHEN trim(both ' ' from trim(both '/' from obj)) = '' THEN 'policy'
        ELSE trim(both ' ' from trim(both '/' from obj))
    END || ' • ' ||
    CASE
        WHEN trim(both ' ' from act) = '' THEN 'ANY'
        ELSE trim(both ' ' from act)
    END
)
WHERE name IS NULL OR name = '';
