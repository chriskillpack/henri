package main

import (
	"context"
	"fmt"
	"time"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
	"github.com/schollz/progressbar/v3"
)

// dotp computes the unnormalized dot-product between two vectors. It assumes
// that a and b are equal length.
func dotp(a, b []float32) float32 {
	var sum float64
	for i := range len(a) {
		sum += float64(a[i]) * float64(b[i])
	}

	return float32(sum)
}

func computeCosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0.0, fmt.Errorf("embeddings are different lengths, %d and %d", len(a), len(b))
	}

	// Compute the dot product of the two vectors
	dot := dotp(a, b)

	// Compute the magnitudes of the two vectors
	ma := dotp(a, a)
	mb := dotp(b, b)
	if ma < 1e-6 || mb < 1e-6 {
		return 0, nil
	}

	return dot / (ma * mb), nil
}

func runQuery(query string, d describer.Describer, db *henri.DB) error {
	ctx := context.Background()

	// First things first, convert the query into an embedding queryvec
	fmt.Printf("Computing query embedding vector...\n")
	queryvec, err := d.Embeddings(ctx, query)
	if err != nil {
		return err
	}

	// Get a count of the number of embeddings that match this model
	eids, err := db.EmbeddingIdsForModel(ctx, d.Model())
	if err != nil {
		return err
	}

	bar := progressbar.NewOptions(
		len(eids),
		progressbar.OptionSetDescription("Computing similarities"),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	)

	top5 := NewTopKTracker(5)

	// Iterate over the embeddings scoring each one
	var errcnt int
	for _, eid := range eids {
		if errcnt >= 5 {
			fmt.Print("Too many errors, terminating")
			return err
		}

		var embed *henri.Embedding
		embed, err = db.GetEmbedding(ctx, eid)
		if err != nil {
			errcnt++
			continue
		}

		// Compute the cosine similarity of this embeddeding with the query
		// embedding.
		var score float32
		score, err = computeCosineSimilarity(queryvec, embed.Vector)

		top5.ProcessItem(embed, score)

		bar.Add(1)
	}
	bar.Finish()

	// Get top results
	topes := top5.GetTopK()

	// Extract the embed ids
	embedids := make([]int, top5.k)
	for i, es := range topes {
		embedids[i] = es.embed.Id
	}

	embeddings, err := db.GetEmbeddingsWithImages(ctx, embedids...)
	if err != nil {
		return err
	}

	// Iterate over the top 5 again and print out stuff we care about
	for i, es := range topes {
		emb := embeddings[es.embed.Id]

		fmt.Printf("Idx %d    Score=%0.5f\nPath=%q\nDescription=%q\n", i+1, es.score, emb.Image.Path, emb.Image.Description)
		if i < len(topes)-1 {
			fmt.Println("==========")
		}
	}

	return nil
}
