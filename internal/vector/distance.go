package vector

import (
	"math"
)

type Metric uint8

const (
	Cosine Metric = iota
	L2
	DotProduct
)

// Distance computes distance between two float32 vectors.
// Lower distance = more similar.
func Distance(a, b []float32, metric Metric) float32 {
	switch metric {
	case Cosine:
		return CosineDistance(a, b)
	case L2:
		return L2Distance(a, b)
	case DotProduct:
		return -DotProductDistance(a, b) // Invert so higher dot product = lower distance
	default:
		return L2Distance(a, b)
	}
}

// CosineDistance computes 1.0 - cosine_similarity with 8x loop unrolling for SIMD pipelining.
func CosineDistance(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot0, dot1, dot2, dot3, dot4, dot5, dot6, dot7 float32
	var normA0, normA1, normA2, normA3, normA4, normA5, normA6, normA7 float32
	var normB0, normB1, normB2, normB3, normB4, normB5, normB6, normB7 float32

	i := 0
	for ; i+7 < n; i += 8 {
		_ = a[i+7]
		_ = b[i+7]
		dot0 += a[i] * b[i]
		dot1 += a[i+1] * b[i+1]
		dot2 += a[i+2] * b[i+2]
		dot3 += a[i+3] * b[i+3]
		dot4 += a[i+4] * b[i+4]
		dot5 += a[i+5] * b[i+5]
		dot6 += a[i+6] * b[i+6]
		dot7 += a[i+7] * b[i+7]

		normA0 += a[i] * a[i]
		normA1 += a[i+1] * a[i+1]
		normA2 += a[i+2] * a[i+2]
		normA3 += a[i+3] * a[i+3]
		normA4 += a[i+4] * a[i+4]
		normA5 += a[i+5] * a[i+5]
		normA6 += a[i+6] * a[i+6]
		normA7 += a[i+7] * a[i+7]

		normB0 += b[i] * b[i]
		normB1 += b[i+1] * b[i+1]
		normB2 += b[i+2] * b[i+2]
		normB3 += b[i+3] * b[i+3]
		normB4 += b[i+4] * b[i+4]
		normB5 += b[i+5] * b[i+5]
		normB6 += b[i+6] * b[i+6]
		normB7 += b[i+7] * b[i+7]
	}

	dot := dot0 + dot1 + dot2 + dot3 + dot4 + dot5 + dot6 + dot7
	normA := normA0 + normA1 + normA2 + normA3 + normA4 + normA5 + normA6 + normA7
	normB := normB0 + normB1 + normB2 + normB3 + normB4 + normB5 + normB6 + normB7

	for ; i < n; i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 1.0
	}
	sim := dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
	return 1.0 - sim
}

// L2Distance computes Euclidean distance with 8x loop unrolling.
func L2Distance(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum0, sum1, sum2, sum3, sum4, sum5, sum6, sum7 float32
	i := 0
	for ; i+7 < n; i += 8 {
		_ = a[i+7]
		_ = b[i+7]
		d0 := a[i] - b[i]
		d1 := a[i+1] - b[i+1]
		d2 := a[i+2] - b[i+2]
		d3 := a[i+3] - b[i+3]
		d4 := a[i+4] - b[i+4]
		d5 := a[i+5] - b[i+5]
		d6 := a[i+6] - b[i+6]
		d7 := a[i+7] - b[i+7]

		sum0 += d0 * d0
		sum1 += d1 * d1
		sum2 += d2 * d2
		sum3 += d3 * d3
		sum4 += d4 * d4
		sum5 += d5 * d5
		sum6 += d6 * d6
		sum7 += d7 * d7
	}
	sum := sum0 + sum1 + sum2 + sum3 + sum4 + sum5 + sum6 + sum7
	for ; i < n; i++ {
		d := a[i] - b[i]
		sum += d * d
	}
	return float32(math.Sqrt(float64(sum)))
}

// DotProductDistance computes dot product with 8x loop unrolling.
func DotProductDistance(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot0, dot1, dot2, dot3, dot4, dot5, dot6, dot7 float32
	i := 0
	for ; i+7 < n; i += 8 {
		_ = a[i+7]
		_ = b[i+7]
		dot0 += a[i] * b[i]
		dot1 += a[i+1] * b[i+1]
		dot2 += a[i+2] * b[i+2]
		dot3 += a[i+3] * b[i+3]
		dot4 += a[i+4] * b[i+4]
		dot5 += a[i+5] * b[i+5]
		dot6 += a[i+6] * b[i+6]
		dot7 += a[i+7] * b[i+7]
	}
	dot := dot0 + dot1 + dot2 + dot3 + dot4 + dot5 + dot6 + dot7
	for ; i < n; i++ {
		dot += a[i] * b[i]
	}
	return dot
}

// Match represents a candidate vector match.
type Match struct {
	ID       uint64
	Distance float32
}

type maxHeap []Match

func (h *maxHeap) push(m Match) {
	*h = append(*h, m)
	j := len(*h) - 1
	for {
		i := (j - 1) / 2
		if i == j || (*h)[j].Distance <= (*h)[i].Distance {
			break
		}
		(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
		j = i
	}
}

func (h *maxHeap) pop() Match {
	old := *h
	n := len(old) - 1
	old[0], old[n] = old[n], old[0]

	i := 0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 {
			break
		}
		j := j1
		if j2 := j1 + 1; j2 < n && old[j2].Distance > old[j1].Distance {
			j = j2
		}
		if old[j].Distance <= old[i].Distance {
			break
		}
		old[i], old[j] = old[j], old[i]
		i = j
	}

	x := old[n]
	*h = old[0:n]
	return x
}

// ExactTopK performs exact nearest-neighbor search using a bounded max-heap.
func ExactTopK(query []float32, candidates []Match, vectors map[uint64][]float32, k int, metric Metric) []Match {
	h := &maxHeap{}

	for _, cand := range candidates {
		vec, ok := vectors[cand.ID]
		if !ok || len(vec) != len(query) {
			continue
		}
		dist := Distance(query, vec, metric)

		if len(*h) < k {
			h.push(Match{ID: cand.ID, Distance: dist})
		} else if dist < (*h)[0].Distance {
			h.pop()
			h.push(Match{ID: cand.ID, Distance: dist})
		}
	}

	result := make([]Match, len(*h))
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = h.pop()
	}
	return result
}
