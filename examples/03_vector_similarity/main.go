package main

import (
	"fmt"
	"log"

	"cocodb/internal/vector"
)

func main() {
	fmt.Println("=== CoCo Vector Search (HNSW Graph) ===")

	// 1. Initialize HNSW index with Cosine Distance
	cfg := vector.DefaultHNSWConfig(4)
	cfg.Metric = vector.Cosine
	cfg.EfConstruction = 64
	cfg.EfSearch = 32

	hnsw := vector.NewHNSW(cfg)

	// 2. Insert items with 4-dimensional vector embeddings
	items := []struct {
		id     uint64
		label  string
		vector []float32
	}{
		{id: 1, label: "Deep Learning Book", vector: []float32{0.9, 0.1, 0.2, 0.05}},
		{id: 2, label: "Machine Learning Paper", vector: []float32{0.85, 0.15, 0.18, 0.08}},
		{id: 3, label: "Database Systems Architecture", vector: []float32{0.1, 0.88, 0.75, 0.12}},
		{id: 4, label: "SQL & Transaction Isolation", vector: []float32{0.12, 0.92, 0.80, 0.09}},
		{id: 5, label: "Gourmet Coffee Brewing", vector: []float32{0.05, 0.02, 0.1, 0.95}},
	}

	for _, item := range items {
		err := hnsw.Insert(item.id, item.vector)
		if err != nil {
			log.Fatalf("Failed to insert vector #%d: %v", item.id, err)
		}
		fmt.Printf("Indexed item #%d: %-30s | Vec: %v\n", item.id, item.label, item.vector)
	}

	// 3. Search for Top-2 nearest neighbors to an AI/ML query vector
	queryVec := []float32{0.95, 0.12, 0.15, 0.02} // AI / Deep Learning query
	fmt.Printf("\nSearching Top-2 nearest neighbors for query: %v\n", queryVec)

	results := hnsw.Search(queryVec, 2)
	for i, match := range results {
		var label string
		for _, item := range items {
			if item.id == match.ID {
				label = item.label
				break
			}
		}
		simPct := (1.0 - match.Distance) * 100.0
		fmt.Printf("  #%d -> Item %d: %-30s | Cosine Dist: %.4f (Similarity: %.1f%%)\n",
			i+1, match.ID, label, match.Distance, simPct)
	}

	// 4. Search for Top-2 nearest neighbors to a Database query vector
	dbQuery := []float32{0.08, 0.95, 0.82, 0.10} // Database systems query
	fmt.Printf("\nSearching Top-2 nearest neighbors for query: %v\n", dbQuery)

	results = hnsw.Search(dbQuery, 2)
	for i, match := range results {
		var label string
		for _, item := range items {
			if item.id == match.ID {
				label = item.label
				break
			}
		}
		simPct := (1.0 - match.Distance) * 100.0
		fmt.Printf("  #%d -> Item %d: %-30s | Cosine Dist: %.4f (Similarity: %.1f%%)\n",
			i+1, match.ID, label, match.Distance, simPct)
	}
}
