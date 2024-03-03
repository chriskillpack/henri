CREATE TABLE migrations (
    id INTEGER NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE images (
    id INTEGER NOT NULL PRIMARY KEY,
    image_path TEXT UNIQUE NOT NULL,
    image_mtime TIMESTAMPTZ NOT NULL,
    image_description TEXT,
    processed_at TIMESTAMPTZ,
    attempted_at TIMESTAMPTZ
);