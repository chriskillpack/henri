package henri

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tailscale/squibble"
	_ "modernc.org/sqlite"
)

//go:embed db/latest_schema.sql
var dbSchema string

var schema = &squibble.Schema{
	Current: dbSchema,

	Updates: []squibble.UpdateRule{
		{
			Source: "5a9671c04c8b8a28c0d7d7ff6ad328f1890c7e3b891bffb0f77f9a966ed51978",
			Target: "483128f2721d69153684ba823861680c7c534ae548a3a8a1010d1372d8c7c58c",
			Apply: squibble.Exec(
				`DROP TABLE IF EXISTS migrations`,
			),
		},

		{
			Source: "483128f2721d69153684ba823861680c7c534ae548a3a8a1010d1372d8c7c58c",
			Target: "fd6688375b27315dc86feda5caa174bbde47205a0485eb3ff23f34c4e17d573f",
			Apply: squibble.Exec(
				`ALTER TABLE images ADD describer VARCHAR`,
			),
		},

		// This is an intentional no-op. The DB went through some manual transformation
		// to get it to match the schema described in db/latest_schema.sql and this entry
		// is necessary to keep the migration system happy.
		{
			Source: "fd6688375b27315dc86feda5caa174bbde47205a0485eb3ff23f34c4e17d573f",
			Target: "7c454dd40e25ac4458b7378da3d5087378f378bfb5023abbb24ea1ba2fef17cd",
			Apply: squibble.Exec(
				`SELECT 1=1`,
			),
		},
	},
}

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
	ProcessedAt sql.NullTime
	AttemptedAt sql.NullTime
}

func (db *DB) Close() {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.db.Close()
}

func NewDB(ctx context.Context, fname string) (*DB, error) {
	// Open the DB but flip on the cleaner timestamps from Go
	sqldb, err := sql.Open("sqlite", fname+"?_time_format=sqlite")
	if err != nil {
		return nil, err
	}
	if err := sqldb.PingContext(ctx); err != nil {
		return nil, err
	}
	if err := schema.Apply(ctx, sqldb); err != nil {
		return nil, err
	}

	return &DB{db: sqldb, filepath: fname}, nil
}

func (db *DB) InsertImagePaths(ctx context.Context, filepaths []string, mtimes []time.Time, batchSize int) (int, error) {
	// const batchSize = 100

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

			values = append(values, filepaths[start+idx], mtimes[start+idx])
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

func (db *DB) InsertImagePathsSingle(ctx context.Context, filepaths []string, mtimes []time.Time) error {
	if len(filepaths) != len(mtimes) {
		return fmt.Errorf("filepaths and mtimes lengths do not match")
	}

	for i := range filepaths {
		if _, err := db.db.ExecContext(ctx, "INSERT OR IGNORE INTO images (image_path, image_mtime) VALUES (?,?)", filepaths[i], mtimes[i]); err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) InsertImagePathsSingleTxn(ctx context.Context, filepaths []string, mtimes []time.Time) error {
	if len(filepaths) != len(mtimes) {
		return fmt.Errorf("filepaths and mtimes lengths do not match")
	}

	txn, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer txn.Rollback()

	for i := range filepaths {
		if _, err := txn.ExecContext(ctx, "INSERT OR IGNORE INTO images (image_path, image_mtime) VALUES (?,?)", filepaths[i], mtimes[i]); err != nil {
			return err
		}
	}

	return txn.Commit()
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

		var desc sql.NullString
		err = rows.Scan(&img.Id, &img.Path, &img.PathMTime, &desc)
		if err != nil {
			return nil, err
		}
		if desc.Valid {
			img.Description = desc.String
		}
		images = append(images, img)
	}

	return images, nil
}

func (db *DB) UpdateImage(ctx context.Context, img *Image, describer string) error {
	_, err := db.db.ExecContext(ctx,
		"UPDATE images SET image_description=$1,describer=$2,processed_at=$3 WHERE id=$4",
		img.Description,
		describer,
		img.ProcessedAt,
		img.Id)
	return err
}

func (db *DB) UpdateImageAttempted(ctx context.Context, id int, describer string, at time.Time) error {
	_, err := db.db.ExecContext(ctx,
		"UPDATE images SET attempted_at=$1,describer=$2 WHERE id=$3",
		at,
		describer,
		id)
	return err
}
