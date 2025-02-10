CREATE TABLE images (
    id INTEGER NOT NULL PRIMARY KEY,
    image_path TEXT UNIQUE NOT NULL,
    image_mtime TIMESTAMP NOT NULL,
    image_description TEXT,
    processed_at TIMESTAMP,
    attempted_at TIMESTAMP,
    describer VARCHAR
);

CREATE TABLE embeddings (
    id INTEGER NOT NULL PRIMARY KEY,
    image_id INTEGER NOT NULL,
    vector BLOB,
    processed_at TIMESTAMP
);

CREATE VIEW embeds AS
SELECT
    id,
    image_id,
    length(vector) AS lenvec,
    processed_at
FROM
    embeddings;
