package henri

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/binary"
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

		{
			Source: "7c454dd40e25ac4458b7378da3d5087378f378bfb5023abbb24ea1ba2fef17cd",
			Target: "427f25914892f62a58a8e4bdf5a798be94a6ef360e09079433f5044278cd7e16",
			Apply: squibble.Exec(
				`CREATE TABLE embeddings (
					id INTEGER NOT NULL PRIMARY KEY,
					image_id INTEGER NOT NULL,
					vector BLOB,
					processed_at TIMESTAMP
				)`,
			),
		},

		{
			Source: "427f25914892f62a58a8e4bdf5a798be94a6ef360e09079433f5044278cd7e16",
			Target: "3b6b01fbac91682c5be525d99f0cef37cfc57c1909e69099715c2eea34ccde67",
			Apply: squibble.Exec(
				`CREATE VIEW embeds AS
				SELECT
					id,
					image_id,
					length(vector) AS lenvec,
					processed_at
				FROM
					embeddings;`,
			),
		},

		{
			Source: "3b6b01fbac91682c5be525d99f0cef37cfc57c1909e69099715c2eea34ccde67",
			Target: "1ac7bf721d8b6c1c9b85ce19aebef975cb833ba79d9c0619c5e0a159d68c7106",
			Apply: squibble.Exec(
				`ALTER TABLE images ADD COLUMN model VARCHAR;`,
				`CREATE UNIQUE INDEX images_image_path_model_index
				 ON images(image_path,model);`,
				`ALTER TABLE embeddings ADD COLUMN model VARCHAR;`,
				`CREATE UNIQUE INDEX embeddings_image_id_model_index
				 ON embeddings(image_id,model);`,
				`DROP VIEW embeds;`,
				`CREATE VIEW embeds AS
				SELECT
					id,
					image_id,
					length(vector) AS lenvec,
					model,
					processed_at
				FROM
					embeddings;`,
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
	Model       string
	Describer   string

	Embedding *Embedding // optional reference
}

type Embedding struct {
	Id          int
	ImageId     int
	Vector      []float32
	Model       string
	ProcessedAt time.Time

	Image *Image // parent image
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

// Only used in benchmarks
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

// Only used in benchmarks
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

// ImagesToDescribe returns Image models for all the images in the DB that lack
// a description.
func (db *DB) ImagesToDescribe(ctx context.Context) ([]*Image, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id, image_path, image_mtime, image_description
		FROM images
		WHERE processed_at IS NULL AND attempted_at IS NULL`)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return images, nil
}

// UpdateImage updates the associated row in the images table from the Image
// model. Only the description, describer and processed_at columns are updated,
// hence this function should be called after a successful image description has
// been generated.
func (db *DB) UpdateImage(ctx context.Context, img *Image, model, describer string) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE images SET image_description=$1,model=$2,describer=$3,
				  processed_at=$4
		WHERE id=$5`,
		img.Description,
		model,
		describer,
		img.ProcessedAt,
		img.Id)
	return err
}

// UpdateImageAttempted updates the attempted_at timestamp for an images row.
func (db *DB) UpdateImageAttempted(ctx context.Context, id int, model, describer string, at time.Time) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE images SET attempted_at=$1,model=$2,describer=$3
		WHERE id=$4`,
		at,
		model,
		describer,
		id)
	return err
}

// DescribedImagesMissingEmbeddings finds all described images that do not have
// a description embedding generated by the specified model. It returns the
// images as Image models, with their Embedding associations blank.
func (db *DB) DescribedImagesMissingEmbeddings(ctx context.Context, model string) ([]*Image, error) {
	query := `
		SELECT i.id, i.image_path, i.image_mtime, i.image_description,
		       i.processed_at, i.attempted_at, i.model, i.describer
		FROM images i
		LEFT JOIN embeddings e ON i.id=e.image_id AND e.model=$1
		WHERE i.image_description IS NOT NULL AND e.id IS NULL`

	rows, err := db.db.QueryContext(ctx, query, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []*Image
	for rows.Next() {
		img := &Image{}

		var desc sql.NullString
		err := rows.Scan(
			&img.Id,
			&img.Path,
			&img.PathMTime,
			&desc,
			&img.ProcessedAt,
			&img.AttemptedAt,
			&img.Model,
			&img.Describer,
		)
		if err != nil {
			return nil, err
		}
		if desc.Valid {
			img.Description = desc.String
		}

		images = append(images, img)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return images, nil
}

// CreateEmbedding inserts a new row into the embedding table and returns an
// Embedding model
func (db *DB) CreateEmbedding(ctx context.Context, vector []float32, model string, img *Image, at time.Time) (*Embedding, error) {
	embed := &Embedding{
		ImageId:     img.Id,
		Vector:      vector,
		Model:       model,
		ProcessedAt: at,
		Image:       img,
	}
	buf := &bytes.Buffer{}
	buf.Grow(len(vector) * 4)
	if err := binary.Write(buf, binary.BigEndian, vector); err != nil {
		return nil, err
	}

	// Insert the embedding
	res, err := db.db.ExecContext(ctx, `
		INSERT INTO embeddings (image_id, vector, model, processed_at)
		VALUES ($1,$2,$3,$4)`,
		img.Id, buf.Bytes(), model, at,
	)
	if err != nil {
		return nil, err
	}
	// Update the model's id
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	embed.Id = int(id)

	// Update the Image's association to this embedding
	img.Embedding = embed
	return embed, nil
}

// GetEmbedding retrieves an Embedding model for the given embedding ID.
// Currently this does not set up the Image association on the returned
// Embedding.
func (db *DB) GetEmbedding(ctx context.Context, id int) (*Embedding, error) {
	row := db.db.QueryRowContext(ctx, `
		SELECT id, image_id, vector, model, processed_at
		FROM embeddings
		WHERE id=$1`, id)
	if row.Err() != nil {
		return nil, row.Err()
	}

	var blobData []byte
	embed := &Embedding{}
	err := row.Scan(
		&embed.Id,
		&embed.ImageId,
		&blobData,
		&embed.Model,
		&embed.ProcessedAt,
	)
	if err != nil {
		return nil, err
	}

	embed.Vector = make([]float32, len(blobData)/4)
	err = binary.Read(bytes.NewReader(blobData), binary.BigEndian, &embed.Vector)
	if err != nil {
		return nil, err
	}
	return embed, nil
}

// EmbeddingBatch holds a results batch for EmbeddingsForModel.
type EmbeddingBatch struct {
	Embeds     []*Embedding // len <= EmbeddingsForModel batchSize param
	LastIDSeen int
	Done       bool
}

// EmbeddingsForModel returns Embedding for model. It is a batching API so it
// returns a channel that will receive batches of Embeddings. The last batch
// will set Done to true and the channel will be closed. Cancel the supplied
// context to terminate the batching process.
func (db *DB) EmbeddingsForModel(ctx context.Context, model string, batchSize int) (<-chan EmbeddingBatch, <-chan error) {
	if batchSize == 0 {
		batchSize = 1000
	}

	batchChan := make(chan EmbeddingBatch)
	errChan := make(chan error, 1)

	go func() {
		defer close(batchChan)
		defer close(errChan)

		var lastID int
		for {
			if ctx.Err() != nil {
				errChan <- ctx.Err()
				return
			}

			batch, err := db.loadEmbeddingsForBatch(ctx, model, batchSize, lastID)
			if err != nil {
				errChan <- fmt.Errorf("loading embedding batch - %w", err)
				return
			}

			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case batchChan <- batch:
			}

			if batch.Done {
				return
			}

			lastID = batch.LastIDSeen
		}
	}()

	return batchChan, errChan
}

func (db *DB) loadEmbeddingsForBatch(ctx context.Context, model string, batchSize, lastID int) (EmbeddingBatch, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT e.id, e.image_id, e.vector, e.processed_at,
		       i.id, i.image_path, i.image_mtime, i.image_description, i.processed_at, i.attempted_at,
		       i.describer, i.model
		FROM embeddings e
		INNER JOIN images i ON e.image_id=i.id
		WHERE e.model=$1 AND e.id > $2
		ORDER BY e.id
		LIMIT $3`, model, lastID, batchSize)
	if err != nil {
		return EmbeddingBatch{}, fmt.Errorf("querying embeddings - %w", err)
	}
	defer rows.Close()

	batch := EmbeddingBatch{}
	batch.Embeds = make([]*Embedding, 0, batchSize)
	var blobData []byte
	for rows.Next() {
		var img Image
		emb := &Embedding{Model: model, Image: &img}
		err := rows.Scan(
			&emb.Id,
			&emb.ImageId,
			&blobData,
			&emb.ProcessedAt,
			&img.Id,
			&img.Path,
			&img.PathMTime,
			&img.Description,
			&img.ProcessedAt,
			&img.AttemptedAt,
			&img.Describer,
			&img.Model,
		)
		if err != nil {
			return EmbeddingBatch{}, fmt.Errorf("scanning rows - %w", err)
		}

		emb.Vector = make([]float32, len(blobData)/4)
		err = binary.Read(bytes.NewReader(blobData), binary.BigEndian, &emb.Vector)
		if err != nil {
			return EmbeddingBatch{}, fmt.Errorf("reading vector data - %w", err)
		}

		batch.Embeds = append(batch.Embeds, emb)
		batch.LastIDSeen = emb.Id
	}
	if rows.Err() != nil {
		return EmbeddingBatch{}, fmt.Errorf("scanning rows - %w", err)
	}

	batch.Done = len(batch.Embeds) < batchSize
	return batch, nil
}

// EmbeddingIdsForModel returns the the ids of all embeddings that match a model
func (db *DB) EmbeddingIdsForModel(ctx context.Context, model string) ([]int, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id
		FROM embeddings
		WHERE model=$1`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	eids := make([]int, 0, 1024)
	for rows.Next() {
		var eid int
		if err := rows.Scan(&eid); err != nil {
			return nil, err
		}
		eids = append(eids, eid)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return eids, nil
}

// GetEmbeddingsWithImages looks up embeddings by id and returns both the embed
// (without vector data) and the associated Image.
func (db *DB) GetEmbeddingsWithImages(ctx context.Context, ids ...int) (map[int]*Embedding, error) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT e.id,e.image_id,e.model,e.processed_at,
		       i.id,i.image_path,i.image_mtime,i.image_description,
		       i.processed_at,i.attempted_at,i.model,i.describer
		FROM embeds e
		INNER JOIN images i ON e.image_id=i.id
		WHERE e.id IN (%s)`,
		strings.Join(placeholders, ","))

	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	embeddings := make(map[int]*Embedding)
	for rows.Next() {
		emb := &Embedding{}
		img := &Image{}

		err := rows.Scan(
			&emb.Id,
			&emb.ImageId,
			&emb.Model,
			&emb.ProcessedAt,
			&img.Id,
			&img.Path,
			&img.PathMTime,
			&img.Description,
			&img.ProcessedAt,
			&img.AttemptedAt,
			&img.Model,
			&img.Describer,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning embeddings and images: %w", err)
		}

		img.Embedding = emb
		emb.Image = img
		embeddings[emb.Id] = emb
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating embeddings and images: %w", err)
	}

	return embeddings, nil
}
