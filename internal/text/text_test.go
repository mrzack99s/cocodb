package text_test

import (
	"testing"

	"github.com/mrzack99s/cocodb/internal/text"
)

func TestTokenizerAndBM25(t *testing.T) {
	tokens := text.Tokenize("CoCo Database is a High-Performance embedded engine in Pure Go!")
	if len(tokens) < 7 {
		t.Fatalf("Tokenize returned too few tokens: %v", tokens)
	}
	if tokens[0] != "coco" || tokens[1] != "database" {
		t.Fatalf("Tokenize unexpected tokens: %v", tokens)
	}

	idx := text.NewInvertedIndex()
	idx.IndexDoc(1, "Go embedded database for high performance applications")
	idx.IndexDoc(2, "Recipes for chocolate chip cookies and baking dessert")
	idx.IndexDoc(3, "High performance vector database in pure Go")

	results := idx.Search("embedded database Go", 5)
	if len(results) == 0 {
		t.Fatalf("Search returned no results")
	}

	// Document 1 should rank #1 because it contains all 3 terms
	if results[0].RecordID != 1 {
		t.Fatalf("expected doc 1 to rank highest, got doc %d", results[0].RecordID)
	}

	// Delete doc 1 and search again
	idx.DeleteDoc(1)
	resultsAfter := idx.Search("embedded database Go", 5)
	for _, res := range resultsAfter {
		if res.RecordID == 1 {
			t.Fatalf("doc 1 still returned after deletion")
		}
	}
}
