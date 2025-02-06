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
