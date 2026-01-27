-- name: GetHash :one
SELECT hash FROM urls
WHERE url = ? AND algo = ? LIMIT 1;

-- name: InsertHash :exec
INSERT OR REPLACE INTO urls (
  url, hash, algo
) VALUES (
  ?, ?, ?
);
