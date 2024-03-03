package henri

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed db/latest_schema.sql
var sqlSchema string

//go:embed db/migrations
var migrationFS embed.FS

type DB struct {
	mu sync.Mutex
	db *sql.DB

	filepath string
}

type Image struct {
	Id          int
	Path        string
	PathMTime   time.Time
	Description string
	ProcessedAt time.Time
	AttemptedAt time.Time
}

func (db *DB) Close() {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.db.Close()
}

func NewDB(ctx context.Context, fname string) (*DB, error) {
	sqldb, err := sql.Open("sqlite", fname)
	if err != nil {
		return nil, err
	}
	if err := sqldb.PingContext(ctx); err != nil {
		return nil, err
	}

	db := &DB{db: sqldb, filepath: fname}
	if err := db.applyMigrations(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) InsertImagePaths(ctx context.Context, filepaths []string, mtimes []time.Time) (int, error) {
	const batchSize = 100

	if len(filepaths) != len(mtimes) {
		return 0, fmt.Errorf("filepaths and mtimes lengths do not match")
	}

	txn, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	start := 0
	affected := 0
	for start < len(filepaths) {
		end := start + batchSize
		if end > len(filepaths) {
			end = len(filepaths)
		}

		qsb := strings.Builder{}
		qsb.WriteString("INSERT OR IGNORE INTO images (image_path, image_mtime) VALUES")
		values := make([]any, 0, batchSize*2)
		for idx := range filepaths[start:end] {
			qsb.WriteString(" ($")
			qsb.WriteString(strconv.Itoa(idx*2 + 1))
			qsb.WriteString(",$")
			qsb.WriteString(strconv.Itoa(idx*2 + 2))
			qsb.WriteString("),")

			values = append(values, filepaths[start+idx], mtimes[start+idx].Format(time.DateTime))
		}
		queryString := qsb.String()

		// Remove trailing comma
		queryString = queryString[0 : len(queryString)-1]

		res, err := txn.ExecContext(ctx, queryString, values...)
		if err != nil {
			return 0, err
		}

		ra, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		affected += int(ra)
		start = end
	}

	return affected, txn.Commit()
}

func (db *DB) ImagesToDescribe(ctx context.Context) ([]*Image, error) {
	rows, err := db.db.QueryContext(
		ctx,
		"SELECT id, image_path, image_mtime, image_description FROM images WHERE processed_at IS NULL AND attempted_at IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []*Image
	for rows.Next() {
		img := &Image{}

		var mtime, desc sql.NullString
		err = rows.Scan(&img.Id, &img.Path, &mtime, &desc)
		if err != nil {
			return nil, err
		}
		if mtime.Valid {
			img.PathMTime, err = time.Parse(time.DateTime, mtime.String)
			if err != nil {
				return nil, err
			}
		}
		if desc.Valid {
			img.Description = desc.String
		}
		images = append(images, img)
	}

	return images, nil
}

func (db *DB) UpdateImage(ctx context.Context, img *Image) error {
	_, err := db.db.ExecContext(ctx,
		"UPDATE images SET image_description=$1,processed_at=$2 WHERE id=$3",
		img.Description,
		img.ProcessedAt.Format(time.DateTime),
		img.Id)
	return err
}

func (db *DB) UpdateImageAttempted(ctx context.Context, id int, at time.Time) error {
	_, err := db.db.ExecContext(ctx,
		"UPDATE images SET attempted_at=$1 WHERE id=$2",
		at.Format(time.DateTime),
		id)
	return err
}

func (db *DB) applyMigrations(ctx context.Context) error {
	migs, err := db.allAppliedMigrations(ctx)
	if err != nil {
		return err
	}
	// If the migrations table doesn't exist (which we infer from migs being nil) then
	// assume that the DB is empty and attempt to apply the latest schema.
	if migs == nil {
		return db.applyLatestSchema(ctx, sqlSchema)
	}

	// Sort the applied migrations in ascending order
	slices.SortFunc(migs, migSort)

	// What is the total set of migrations included?
	fsmigs, err := fs.Glob(migrationFS, "db/migrations/*.sql")
	if err != nil {
		return err
	}
	slices.SortFunc(fsmigs, migSort)

	// If there are migrations to be applied then backup the DB
	if len(fsmigs) > len(migs) {
		rawdb, err := os.ReadFile(db.filepath)
		if err != nil {
			return err
		}
		backupDB := fmt.Sprintf("%s_%d_backup.db", db.filepath, time.Now().Unix())
		if err := os.WriteFile(backupDB, rawdb, 0644); err != nil {
			return err
		}
		fmt.Println("Backed up DB to", backupDB)
	}

	// Apply outstanding migrations
	// migs < fsmigs = Typical case, walk from migs to fsmigs
	// migs == fsmigs = Nothing to be done
	// migs > fsmigs = Nothing to be done, unexpected state
	if len(migs) < len(fsmigs) {
		fmt.Println("Starting migration")
	}
	for i := len(migs); i < len(fsmigs); i++ {
		fmt.Printf("Applying migration %s", fsmigs[i])

		migddl, err := migrationFS.ReadFile(fsmigs[i])
		if err != nil {
			return err
		}
		if err := db.ApplyMigration(ctx, string(migddl), fsmigs[i]); err != nil {
			fmt.Println(", error:", err, "aborting")
			return err
		}
		fmt.Println()
	}
	if len(migs) < len(fsmigs) {
		fmt.Println("Finished migration")
	}

	return nil
}

func migSort(a, b string) int {
	ai, err := strconv.Atoi(strings.Split(a, "_")[0])
	bi, err2 := strconv.Atoi(strings.Split(b, "_")[0])
	if err != nil || err2 != nil {
		panic(fmt.Sprintf("Invalid migration filenames %s,%s", a, b))
	}

	d := ai - bi
	if d == 0 {
		panic("Cannot have migrations with the same ordering key")
	}
	return d
}

func (db *DB) allAppliedMigrations(ctx context.Context) ([]string, error) {
	// Does the schema_migrations table exist?
	row := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='migrations'")
	var success int
	err := row.Scan(&success)
	if err != nil {
		return nil, err
	}
	if success == 0 {
		return nil, nil // should be interpreted as "apply the latest schema directly"
	}

	rows, err := db.db.QueryContext(ctx, "SELECT name FROM migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	migs := make([]string, 0, 10)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		migs = append(migs, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return migs, nil
}

func (db *DB) ApplyMigration(ctx context.Context, ddl string, migname string) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(ddl)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO migrations (name) VALUES ($1)", migname)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) applyLatestSchema(ctx context.Context, schema string) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(schema)
	if err != nil {
		return err
	}

	return tx.Commit()
}
