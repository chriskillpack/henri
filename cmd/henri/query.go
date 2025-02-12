package main

import (
	"container/heap"
	"context"
	"fmt"
	"time"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
	"github.com/schollz/progressbar/v3"
)

type embedscore struct {
	embedId int
	score   float32
}

type MinHeap []embedscore

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i].score < h[j].score }
func (h MinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinHeap) Push(x any) {
	*h = append(*h, x.(embedscore))
}

func (h *MinHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]

	return x
}

// TopKTracker keeps track on the top K scoring items
type TopKTracker struct {
	k    int
	heap MinHeap
}

func NewTopKTracker(k int) *TopKTracker {
	topk := &TopKTracker{
		k:    k,
		heap: make(MinHeap, 0, k),
	}
	heap.Init(&topk.heap)
	return topk
}

func (t *TopKTracker) ProcessItem(index int, score float32) {
	if len(t.heap) < t.k {
		heap.Push(&t.heap, embedscore{index, score})
		return
	}

	if score > t.heap[0].score {
		heap.Pop(&t.heap)
		heap.Push(&t.heap, embedscore{index, score})
	}
}

func (t *TopKTracker) GetTopK() []embedscore {
	tempHeap := make(MinHeap, len(t.heap))
	copy(tempHeap, t.heap)

	// Pop items in ascending order
	result := make([]embedscore, len(tempHeap))
	for i := len(tempHeap) - 1; i >= 0; i-- {
		result[i] = heap.Pop(&tempHeap).(embedscore)
	}
	return result
}

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

	// Get a list of embeddings
	ne, err := db.CountEmbeddings(ctx)
	if err != nil {
		return err
	}

	bar := progressbar.NewOptions(
		ne,
		progressbar.OptionSetDescription("Computing similarities"),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	)

	top5 := NewTopKTracker(5)

	// Iterate over the embeddings scoring each one
	var errcnt int
	for i := range ne {
		if errcnt >= 5 {
			fmt.Print("Too many errors, terminating")
			return err
		}

		var embed *henri.Embedding
		embed, err = db.GetEmbedding(ctx, i)
		if err != nil {
			errcnt++
			continue
		}

		// Compute the cosine similarity of this embeddeding with the query
		// embedding.
		var score float32
		score, err = computeCosineSimilarity(queryvec, embed.Vector)

		top5.ProcessItem(i, score)

		bar.Add(1)
	}
	bar.Finish()

	// Get top results
	topes := top5.GetTopK()

	// Extract the embed ids
	embedids := make([]int, top5.k)
	for i, es := range topes {
		embedids[i] = es.embedId
	}

	embeddings, err := db.GetEmbeddingsWithImages(ctx, embedids...)

	// Iterate over the top 5 again and print out stuff we care about
	for i, es := range topes {
		emb := embeddings[es.embedId]

		fmt.Printf("Idx %d    Score=%0.5f\nPath=%q\nDescription=%q\n", i+1, es.score, emb.Image.Path, emb.Image.Description)
		if i < len(topes)-1 {
			fmt.Println("==========")
		}
	}

	return nil
}
