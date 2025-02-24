package main

import (
	"container/heap"

	"github.com/chriskillpack/henri"
)

type embedscore struct {
	embed *henri.Embedding
	score float32
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

func (t *TopKTracker) ProcessItem(embed *henri.Embedding, score float32) {
	if len(t.heap) < t.k {
		heap.Push(&t.heap, embedscore{embed, score})
		return
	}

	if score > t.heap[0].score {
		heap.Pop(&t.heap)
		heap.Push(&t.heap, embedscore{embed, score})
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
