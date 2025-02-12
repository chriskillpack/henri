CREATE TABLE images (
    id INTEGER NOT NULL PRIMARY KEY,
    image_path TEXT UNIQUE NOT NULL,
    image_mtime TIMESTAMP NOT NULL,
    image_description TEXT,
    processed_at TIMESTAMP,
    attempted_at TIMESTAMP,
    describer VARCHAR,
    model VARCHAR
);

CREATE UNIQUE INDEX images_image_path_model_index
ON images(image_path,model);

CREATE TABLE embeddings (
    id INTEGER NOT NULL PRIMARY KEY,
    image_id INTEGER NOT NULL,
    vector BLOB,
    processed_at TIMESTAMP,
    model VARCHAR
);

CREATE UNIQUE INDEX embeddings_image_id_model_index
ON embeddings(image_id,model);

CREATE VIEW embeds AS
SELECT
    id,
    image_id,
    length(vector) AS lenvec,
    model,
    processed_at
FROM
    embeddings;
