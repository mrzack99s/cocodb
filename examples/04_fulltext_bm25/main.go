package main

import (
	"fmt"

	"cocodb/search"
)

func main() {
	fmt.Println("=== CoCo Full-Text Search (BM25 Inverted Index) ===")

	idx := search.NewInvertedIndex()

	documents := []struct {
		id   uint64
		body string
	}{
		{
			id:   1,
			body: "CoCo is a modern high-performance embedded multi-model database written in Pure Go with single-file storage.",
		},
		{
			id:   2,
			body: "PostgreSQL and SQLite are widely used relational database management systems.",
		},
		{
			id:   3,
			body: "Vector search and full-text inverted index BM25 enable semantic and keyword searching in CoCo.",
		},
		{
			id:   4,
			body: "Acid transactions, crash recovery with write-ahead logs (WAL), and snapshot isolation MVCC.",
		},
	}

	// 1. Index documents
	for _, doc := range documents {
		idx.IndexDoc(doc.id, doc.body)
		fmt.Printf("Indexed Doc #%d: %.70s...\n", doc.id, doc.body)
	}

	// 2. Perform BM25 Queries
	queries := []string{
		"embedded multi-model database",
		"vector search BM25",
		"crash recovery WAL isolation",
	}

	for _, query := range queries {
		fmt.Printf("\nQuery: %q\n", query)
		results := idx.Search(query, 3)

		for i, res := range results {
			var body string
			for _, d := range documents {
				if d.id == res.RecordID {
					body = d.body
					break
				}
			}
			fmt.Printf("  #%d -> Doc #%d (BM25 Score: %.3f)\n       %s\n",
				i+1, res.RecordID, res.Score, body)
		}
	}
}
