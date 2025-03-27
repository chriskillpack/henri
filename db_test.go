package henri

import (
	"fmt"
	"testing"
	"time"
)

func TestInsertImagePaths(t *testing.T) {
	db, err := NewDB(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t.Run("empty slice", func(t *testing.T) {
		affected, err := db.InsertImagePaths(t.Context(), []ImagePath{}, 100)
		if err != nil {
			t.Errorf("Unexpected error %s", err)
		}
		if expected, actual := 0, affected; expected != actual {
			t.Errorf("Expected %d rows affected, got %d", expected, actual)
		}
	})

	t.Run("single batch", func(t *testing.T) {
		imgs := []ImagePath{
			{Path: "/path/to/1", Modtime: time.Now(), Width: 640, Height: 480},
			{Path: "/path/to/2", Modtime: time.Now(), Width: 800, Height: 600},
			{Path: "/path/to/3", Modtime: time.Now(), Width: 1024, Height: 768},
		}
		affected, err := db.InsertImagePaths(t.Context(), imgs, 100)
		if err != nil {
			t.Errorf("Unexpected error %s", err)
		}
		if expected, actual := 3, affected; expected != actual {
			t.Errorf("Expected %d rows affected, got %d", expected, actual)
		}
	})

	t.Run("multiple batches", func(t *testing.T) {
		_, err := db.db.ExecContext(t.Context(), "DELETE FROM images")
		if err != nil {
			t.Errorf("Unexpected error %s", err)
		}

		paths := make([]ImagePath, 25)
		for i := range paths {
			paths[i] = ImagePath{
				Path:    fmt.Sprintf("/path/to/%d.jpg", i+1),
				Modtime: time.Now(),
				Width:   1024 + i,
				Height:  768 + i,
			}
		}

		affected, err := db.InsertImagePaths(t.Context(), paths, 10)
		if err != nil {
			t.Errorf("Unexpected error %s", err)
		}
		if expected, actual := 25, affected; expected != actual {
			t.Errorf("Expected %d modified rows, got %d", expected, actual)
		}
	})
}
