-- name: GetHash :one
SELECT hash FROM urls
WHERE url = ? AND algo = ? LIMIT 1;

-- name: GetAllHashes :many
SELECT algo, hash FROM urls
WHERE url = ?
ORDER BY
    CASE algo
        WHEN 'sha256' THEN 1
        WHEN 'sha512' THEN 2
        WHEN 'sha1' THEN 3
        ELSE 4
    END ASC;

-- name: InsertHash :exec
INSERT OR REPLACE INTO urls (
  url, hash, algo
) VALUES (
  ?, ?, ?
);
