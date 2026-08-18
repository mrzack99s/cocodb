package text

import (
	"math"
	"sort"
	"sync"
)

type Posting struct {
	RecordID uint64
	TF       uint32
}

type SearchResult struct {
	RecordID uint64
	Score    float64
}

type srHeap []SearchResult

func (h *srHeap) push(r SearchResult) {
	*h = append(*h, r)
	j := len(*h) - 1
	for {
		i := (j - 1) / 2
		if i == j || (*h)[j].Score >= (*h)[i].Score {
			break
		}
		(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
		j = i
	}
}

func (h *srHeap) pop() SearchResult {
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
		if j2 := j1 + 1; j2 < n && old[j2].Score < old[j1].Score {
			j = j2
		}
		if old[j].Score >= old[i].Score {
			break
		}
		old[i], old[j] = old[j], old[i]
		i = j
	}

	x := old[n]
	*h = old[0:n]
	return x
}

// InvertedIndex manages full-text indexing and BM25 ranking.
type InvertedIndex struct {
	mu           sync.RWMutex
	postings     map[string][]Posting
	docLengths   map[uint64]int
	docTerms     map[uint64][]string
	totalDocLen  int64
	totalDocNum  int64
	k1           float64
	b            float64
}

func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		postings:   make(map[string][]Posting),
		docLengths: make(map[uint64]int),
		docTerms:   make(map[uint64][]string),
		k1:         1.2,
		b:          0.75,
	}
}

// IndexDoc tokenizes and indexes text content for recordID.
func (idx *InvertedIndex) IndexDoc(recID uint64, text string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return
	}

	// Calculate term frequencies for this doc
	tfMap := make(map[string]uint32)
	for _, tok := range tokens {
		tfMap[tok]++
	}

	// Update document length statistics
	docLen := len(tokens)
	if oldLen, exists := idx.docLengths[recID]; exists {
		idx.totalDocLen -= int64(oldLen)
	} else {
		idx.totalDocNum++
	}
	idx.docLengths[recID] = docLen
	idx.totalDocLen += int64(docLen)

	// Add to posting lists
	termList := make([]string, 0, len(tfMap))
	for term, tf := range tfMap {
		termList = append(termList, term)
		postings := idx.postings[term]
		filtered := postings[:0]
		for _, p := range postings {
			if p.RecordID != recID {
				filtered = append(filtered, p)
			}
		}
		filtered = append(filtered, Posting{RecordID: recID, TF: tf})
		idx.postings[term] = filtered
	}
	idx.docTerms[recID] = termList
}

// DeleteDoc removes a document from the index.
func (idx *InvertedIndex) DeleteDoc(recID uint64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if docLen, exists := idx.docLengths[recID]; exists {
		idx.totalDocLen -= int64(docLen)
		idx.totalDocNum--
		delete(idx.docLengths, recID)
	}

	terms, hasTerms := idx.docTerms[recID]
	if !hasTerms {
		return
	}
	for _, term := range terms {
		postings := idx.postings[term]
		filtered := postings[:0]
		for _, p := range postings {
			if p.RecordID != recID {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			delete(idx.postings, term)
		} else {
			idx.postings[term] = filtered
		}
	}
	delete(idx.docTerms, recID)
}

// Search calculates BM25 scores for query and returns top-K ranked documents.
func (idx *InvertedIndex) Search(query string, k int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tokens := Tokenize(query)
	if len(tokens) == 0 || idx.totalDocNum == 0 {
		return nil
	}

	avgDL := float64(idx.totalDocLen) / float64(idx.totalDocNum)
	if avgDL == 0 {
		avgDL = 1.0
	}
	N := float64(idx.totalDocNum)
	k1Plus1 := idx.k1 + 1.0
	k1OneMinusB := idx.k1 * (1.0 - idx.b)
	k1BOverAvgDL := (idx.k1 * idx.b) / avgDL

	scores := make(map[uint64]float64)

	for _, term := range tokens {
		postings, ok := idx.postings[term]
		if !ok || len(postings) == 0 {
			continue
		}

		df := float64(len(postings))
		// BM25 IDF
		idf := math.Log(1.0 + (N - df + 0.5)/(df + 0.5))
		idfK1Plus1 := idf * k1Plus1

		for _, p := range postings {
			tf := float64(p.TF)
			docLen := float64(idx.docLengths[p.RecordID])
			denom := tf + k1OneMinusB + k1BOverAvgDL*docLen
			scores[p.RecordID] += (tf * idfK1Plus1) / denom
		}
	}

	if k <= 0 {
		results := make([]SearchResult, 0, len(scores))
		for recID, score := range scores {
			results = append(results, SearchResult{RecordID: recID, Score: score})
		}
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score // Highest score first
		})
		return results
	}

	h := &srHeap{}
	for recID, score := range scores {
		if len(*h) < k {
			h.push(SearchResult{RecordID: recID, Score: score})
		} else if score > (*h)[0].Score {
			h.pop()
			h.push(SearchResult{RecordID: recID, Score: score})
		}
	}

	results := make([]SearchResult, len(*h))
	for i := len(results) - 1; i >= 0; i-- {
		results[i] = h.pop()
	}

	return results
}
