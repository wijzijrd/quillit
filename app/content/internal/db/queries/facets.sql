-- name: ListGlobalFacetNames :many
SELECT name FROM facets ORDER BY name;

-- name: EffectiveFacetVocabulary :many
SELECT name FROM facets
UNION
SELECT name FROM project_facets WHERE project_id = ?
ORDER BY name;

-- name: InsertGlobalFacet :execrows
INSERT OR IGNORE INTO facets (name) VALUES (?);

-- name: DeleteGlobalFacet :exec
DELETE FROM facets WHERE name = ?;

-- name: InsertProjectFacet :exec
INSERT OR IGNORE INTO project_facets (project_id, name) VALUES (?, ?);

-- name: DeleteProjectFacet :exec
DELETE FROM project_facets WHERE project_id = ? AND name = ?;
