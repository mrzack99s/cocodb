package vector_test

import (
	"math/rand"
	"testing"

	"github.com/mrzack99s/cocodb/internal/vector"
)

func TestVectorDistancesAndHNSW(t *testing.T) {
	// 1. Distance function sanity tests
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{0.0, 1.0, 0.0}
	v3 := []float32{1.0, 0.0, 0.0}

	cos12 := vector.CosineDistance(v1, v2)
	cos13 := vector.CosineDistance(v1, v3)
	if cos12 <= cos13 {
		t.Fatalf("orthogonal vectors should have higher distance than identical vectors")
	}

	l2_12 := vector.L2Distance(v1, v2)
	l2_13 := vector.L2Distance(v1, v3)
	if l2_12 <= l2_13 || l2_13 != 0.0 {
		t.Fatalf("l2 distance mismatch")
	}

	// 2. HNSW Search vs Exact Top-K
	cfg := vector.DefaultHNSWConfig(16)
	cfg.Metric = vector.Cosine
	hnsw := vector.NewHNSW(cfg)

	r := rand.New(rand.NewSource(42))
	dims := 16
	numVectors := 100

	vectors := make(map[uint64][]float32)
	var candidates []vector.Match

	for i := 1; i <= numVectors; i++ {
		vec := make([]float32, dims)
		for d := 0; d < dims; d++ {
			vec[d] = r.Float32()
		}
		vectors[uint64(i)] = vec
		candidates = append(candidates, vector.Match{ID: uint64(i)})
		if err := hnsw.Insert(uint64(i), vec); err != nil {
			t.Fatalf("hnsw insert %d failed: %v", i, err)
		}
	}

	// Query vector
	queryVec := make([]float32, dims)
	for d := 0; d < dims; d++ {
		queryVec[d] = r.Float32()
	}

	k := 5
	exactResults := vector.ExactTopK(queryVec, candidates, vectors, k, vector.Cosine)
	hnswResults := hnsw.Search(queryVec, k)

	if len(hnswResults) == 0 {
		t.Fatalf("hnsw search returned no results")
	}

	// Check top-1 match
	if hnswResults[0].ID != exactResults[0].ID {
		t.Logf("HNSW top-1 ID %d vs exact %d (approximate ANN)", hnswResults[0].ID, exactResults[0].ID)
	}
}
