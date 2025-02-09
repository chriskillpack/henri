CREATE TABLE images (
    id INTEGER NOT NULL PRIMARY KEY,
    image_path TEXT UNIQUE NOT NULL,
    image_mtime TIMESTAMP NOT NULL,
    image_description TEXT,
    processed_at TIMESTAMP,
    attempted_at TIMESTAMP,
    describer VARCHAR
);