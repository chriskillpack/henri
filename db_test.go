package henri

import (
	"context"
	"testing"
	"time"
)

const count = 40000

func benchmarkInsertImagePaths(b *testing.B, batchSize int) {
	ctx := context.Background()
	db, err := NewDB(ctx, ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		filepaths := make([]string, count)
		mtimes := make([]time.Time, count)

		db.InsertImagePaths(ctx, filepaths, mtimes, batchSize)
	}
}

func BenchmarkInsertImagePaths50(b *testing.B)   { benchmarkInsertImagePaths(b, 50) }
func BenchmarkInsertImagePaths100(b *testing.B)  { benchmarkInsertImagePaths(b, 100) }
func BenchmarkInsertImagePaths500(b *testing.B)  { benchmarkInsertImagePaths(b, 500) }
func BenchmarkInsertImagePaths1000(b *testing.B) { benchmarkInsertImagePaths(b, 1000) }

func BenchmarkInsertImagePathSingle(b *testing.B) {
	ctx := context.Background()
	db, err := NewDB(ctx, ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		filepaths := make([]string, count)
		mtimes := make([]time.Time, count)

		db.InsertImagePathsSingle(ctx, filepaths, mtimes)
	}
}

func BenchmarkInsertImagePathSingleTxn(b *testing.B) {
	ctx := context.Background()
	db, err := NewDB(ctx, ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		filepaths := make([]string, count)
		mtimes := make([]time.Time, count)

		db.InsertImagePathsSingleTxn(ctx, filepaths, mtimes)
	}
}
