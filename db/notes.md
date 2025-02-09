The original table definition was based on a misunderstanding that sqlite supported a TIMESTAMPTZ column type:

```
CREATE TABLE images (
    id INTEGER NOT NULL PRIMARY KEY,
    image_path TEXT UNIQUE NOT NULL,
    image_mtime TIMESTAMPTZ NOT NULL,
    image_description TEXT,
    processed_at TIMESTAMPTZ,
    attempted_at TIMESTAMPTZ,
    describer VARCHAR
);
```

In fact sqlite will take any column type but internally store it as one of a small set of types, see [Datatypes In SQLite](https://www.sqlite.org/datatype3.html) for more information. In our case these columns were internally be stored as TEXT, since the timestamps were strings "2023-01-02 10:11:12".

The sqlite drivers on Go have a convention of checking columns with TEXT storage type and if the declared type matches TIMESTAMP, DATE or DATETIME, then they will try and parse the values into time.Time objects. But because the column type was TIMESTAMPTZ this wasn't happening and sql.Scan would error trying to put a string type into a *time.Time destination.

To fix, the following steps were applied to the DB

```
-- Rename the existing images table
ALTER TABLE images RENAME TO images_old;

-- Create the new images table, note the TIMESTAMP column types
CREATE TABLE images (
    id INTEGER NOT NULL PRIMARY KEY,
    image_path TEXT UNIQUE NOT NULL,
    image_mtime TIMESTAMP NOT NULL,
    image_description TEXT,
    processed_at TIMESTAMP,
    attempted_at TIMESTAMP,
    describer VARCHAR
);

-- Copy data from the old images to the new images
INSERT INTO images (
    id,
    image_path,
    image_mtime,
    image_description,
    processed_at,
    attempted_at,
    describer) SELECT id, image_path, image_mtime, image_description, processed_at, attempted_at, describer FROM images_old;

-- Verify that the tables have identical data (returns TRUE/1 on success)
SELECT NOT EXISTS (SELECT * FROM images
                   EXCEPT
                   SELECT * FROM images_old)
   AND NOT EXISTS (SELECT * FROM images_old
                   EXCEPT
                   SELECT * FROM images);

-- Finally, drop the original table
DROP TABLE images_old;
```

One last piece of the puzzle concerns how the Go sqlite drivers store time.Time values in the DB. They are stored as TEXT columns but they use the String() representation of the time.Time. This representation is a little unsightly as it contains the monotonic time value as well. Instead, the modernc.org driver will use a slightly simpler representation if the DSN parameter `_time_format=sqlite` is used when opening a connection to the DB.

Default time.Time String rep: `2025-02-07 22:55:35.424332 -0800 PST m=+0.008861043`
`_format_time=sqlite` rep: ``2025-02-07 22:55:35.424332-08:00`
