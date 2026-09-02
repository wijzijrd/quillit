-- name: OrphanProjectEntries :execrows
UPDATE entries SET orphaned_at = ? WHERE project_id = ? AND orphaned_at IS NULL;
