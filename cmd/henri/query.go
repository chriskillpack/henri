package main

import (
	"container/heap"
	"context"
	"fmt"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
)

type embedscore struct {
	embedId    int
	similarity float32
}

type MinHeap []embedscore

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i].similarity < h[j].similarity }
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

type TopKTracker struct {
	k    int
	heap MinHeap
}

func NewTopKTracker(k int) *TopKTracker {
	return &TopKTracker{
		k:    k,
		heap: make(MinHeap, 0, k),
	}
}

var counter int

func (t *TopKTracker) ProcessItem(index int, score float32) {
	if len(t.heap) < t.k {
		heap.Push(&t.heap, embedscore{index, score})
		return
	}

	if counter < 3 {
		fmt.Printf("scores in heap\n")
		for _, es := range t.heap {
			fmt.Printf(" %d %0.5f\n", es.embedId, es.similarity)
		}
		counter++
	}

	if score > t.heap[0].similarity {
		heap.Pop(&t.heap)
		heap.Push(&t.heap, embedscore{index, score})
	}
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
	queryvec, err := d.Embeddings(ctx, query)
	if err != nil {
		return err
	}

	// Get a list of embeddings
	ne, err := db.CountEmbeddings(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%d total embeddings\n", ne)

	top5 := NewTopKTracker(5)
	heap.Init(&top5.heap)

	// Iterate over the embeddings scoring each one
	var errcnt int
	for i := range ne {
		if errcnt >= 5 {
			fmt.Print("Too many errors, terminating")
			return err
		}
		if i == 0 || (i%500) == 0 {
			fmt.Printf("Scoring %d/%d... ", i, ne)
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
		// fmt.Printf("%d scores %0.5f\n", i, score)

		top5.ProcessItem(i, score)
	}
	fmt.Printf("Scoring %d/%d\n", ne, ne)

	// Pop everything out
	for top5.heap.Len() > 0 {
		es := top5.heap.Pop().(embedscore)
		fmt.Printf("Score: %0.5f Embed: %d\n", es.similarity, es.embedId)
	}

	return nil
}
