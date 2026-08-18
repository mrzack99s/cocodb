package vector

import (
	"math"
	"math/rand"
	"sync"
)

type HNSWConfig struct {
	M              int
	M0             int
	EfConstruction int
	EfSearch       int
	Metric         Metric
	Dimensions     int
}

func DefaultHNSWConfig(dims int) HNSWConfig {
	return HNSWConfig{
		M:              16,
		M0:             32,
		EfConstruction: 100,
		EfSearch:       50,
		Metric:         Cosine,
		Dimensions:     dims,
	}
}

type Node struct {
	ID        uint64
	Vector    []float32
	Level     int
	Neighbors [][]uint64 // Neighbors[level] = list of neighbor IDs
}

// HNSW represents an in-memory & persistent Hierarchical Navigable Small World graph.
type HNSW struct {
	mu         sync.RWMutex
	cfg        HNSWConfig
	nodes      map[uint64]*Node
	entryPoint uint64
	maxLevel   int
	levelMult  float64
	visitedGen []uint32
	generation uint32
}

// NewHNSW creates a new HNSW index.
func NewHNSW(cfg HNSWConfig) *HNSW {
	if cfg.M <= 0 {
		cfg.M = 16
	}
	if cfg.M0 <= 0 {
		cfg.M0 = cfg.M * 2
	}
	if cfg.EfConstruction <= 0 {
		cfg.EfConstruction = 100
	}
	if cfg.EfSearch <= 0 {
		cfg.EfSearch = 50
	}

	return &HNSW{
		cfg:        cfg,
		nodes:      make(map[uint64]*Node),
		entryPoint: 0,
		maxLevel:   -1,
		levelMult:  1.0 / math.Log(float64(cfg.M)),
		visitedGen: make([]uint32, 0),
		generation: 0,
	}
}

func (h *HNSW) randomLevel() int {
	r := rand.Float64()
	if r == 0 {
		r = 0.0000001
	}
	return int(-math.Log(r) * h.levelMult)
}

// Insert inserts a vector into the HNSW graph.
func (h *HNSW) Insert(id uint64, vec []float32) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	nodeLevel := h.randomLevel()
	node := &Node{
		ID:        id,
		Vector:    vec,
		Level:     nodeLevel,
		Neighbors: make([][]uint64, nodeLevel+1),
	}
	h.nodes[id] = node

	if id >= uint64(len(h.visitedGen)) {
		newLen := id + 1
		if newLen < uint64(len(h.visitedGen)*2) {
			newLen = uint64(len(h.visitedGen) * 2)
		}
		newGen := make([]uint32, newLen)
		copy(newGen, h.visitedGen)
		h.visitedGen = newGen
	}

	if h.maxLevel == -1 {
		// First node in graph
		h.entryPoint = id
		h.maxLevel = nodeLevel
		return nil
	}

	currObj := h.entryPoint
	currDist := Distance(vec, h.nodes[currObj].Vector, h.cfg.Metric)

	// Phase 1: Search top layers down to nodeLevel + 1
	for l := h.maxLevel; l > nodeLevel; l-- {
		changed := true
		for changed {
			changed = false
			for _, neighborID := range h.nodes[currObj].Neighbors[l] {
				nNode := h.nodes[neighborID]
				d := Distance(vec, nNode.Vector, h.cfg.Metric)
				if d < currDist {
					currDist = d
					currObj = neighborID
					changed = true
				}
			}
		}
	}

	// Phase 2: Insert into layers nodeLevel down to 0
	topLayer := nodeLevel
	if h.maxLevel < nodeLevel {
		topLayer = h.maxLevel
	}

	for l := topLayer; l >= 0; l-- {
		candidates := h.searchLayer(vec, currObj, h.cfg.EfConstruction, l)
		m := h.cfg.M
		if l == 0 {
			m = h.cfg.M0
		}

		selected := h.selectNeighbors(candidates, m)
		node.Neighbors[l] = selected

		// Connect bidirectional edges
		for _, neighborID := range selected {
			nNode := h.nodes[neighborID]
			if l < len(nNode.Neighbors) {
				nNode.Neighbors[l] = append(nNode.Neighbors[l], id)
				if len(nNode.Neighbors[l]) > m {
					nNode.Neighbors[l] = h.pruneNeighbors(nNode.Neighbors[l], nNode.Vector, m)
				}
			}
		}

		if len(candidates) > 0 {
			currObj = candidates[0].ID
		}
	}

	if nodeLevel > h.maxLevel {
		h.maxLevel = nodeLevel
		h.entryPoint = id
	}

	return nil
}

// Search finds the top-K closest vectors to query.
func (h *HNSW) Search(query []float32, k int) []Match {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.nodes) == 0 {
		return nil
	}

	currObj := h.entryPoint
	currDist := Distance(query, h.nodes[currObj].Vector, h.cfg.Metric)

	// Greedily zoom down to level 0
	for l := h.maxLevel; l > 0; l-- {
		changed := true
		for changed {
			changed = false
			if l < len(h.nodes[currObj].Neighbors) {
				for _, nID := range h.nodes[currObj].Neighbors[l] {
					d := Distance(query, h.nodes[nID].Vector, h.cfg.Metric)
					if d < currDist {
						currDist = d
						currObj = nID
						changed = true
					}
				}
			}
		}
	}

	ef := h.cfg.EfSearch
	if ef < k {
		ef = k
	}

	candidates := h.searchLayer(query, currObj, ef, 0)
	if len(candidates) > k {
		candidates = candidates[:k]
	}
	return candidates
}

type minCandidateHeap []Match

func (h *minCandidateHeap) push(m Match) {
	*h = append(*h, m)
	j := len(*h) - 1
	for {
		i := (j - 1) / 2
		if i == j || (*h)[j].Distance >= (*h)[i].Distance {
			break
		}
		(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
		j = i
	}
}

func (h *minCandidateHeap) pop() Match {
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
		if j2 := j1 + 1; j2 < n && old[j2].Distance < old[j1].Distance {
			j = j2
		}
		if old[j].Distance >= old[i].Distance {
			break
		}
		old[i], old[j] = old[j], old[i]
		i = j
	}

	x := old[n]
	*h = old[0:n]
	return x
}

func (h *HNSW) searchLayer(query []float32, ep uint64, ef int, level int) []Match {
	h.generation++
	gen := h.generation
	h.visitedGen[ep] = gen

	epDist := Distance(query, h.nodes[ep].Vector, h.cfg.Metric)

	cHeap := &minCandidateHeap{Match{ID: ep, Distance: epDist}}
	wHeap := &maxHeap{Match{ID: ep, Distance: epDist}}

	for len(*cHeap) > 0 {
		curr := cHeap.pop()
		farthest := (*wHeap)[0]

		if curr.Distance > farthest.Distance && len(*wHeap) >= ef {
			break
		}

		if level < len(h.nodes[curr.ID].Neighbors) {
			for _, nID := range h.nodes[curr.ID].Neighbors[level] {
				if h.visitedGen[nID] != gen {
					h.visitedGen[nID] = gen
					nDist := Distance(query, h.nodes[nID].Vector, h.cfg.Metric)

					if nDist < (*wHeap)[0].Distance || len(*wHeap) < ef {
						cHeap.push(Match{ID: nID, Distance: nDist})
						wHeap.push(Match{ID: nID, Distance: nDist})
						if len(*wHeap) > ef {
							wHeap.pop()
						}
					}
				}
			}
		}
	}

	result := make([]Match, len(*wHeap))
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = wHeap.pop()
	}
	return result
}

func (h *HNSW) selectNeighbors(candidates []Match, m int) []uint64 {
	count := len(candidates)
	if count > m {
		count = m
	}
	res := make([]uint64, count)
	for i := 0; i < count; i++ {
		res[i] = candidates[i].ID
	}
	return res
}

func (h *HNSW) pruneNeighbors(neighborIDs []uint64, baseVec []float32, m int) []uint64 {
	type distPair struct {
		id   uint64
		dist float32
	}
	pairs := make([]distPair, 0, len(neighborIDs))
	for _, nID := range neighborIDs {
		nNode, ok := h.nodes[nID]
		if ok {
			pairs = append(pairs, distPair{
				id:   nID,
				dist: Distance(baseVec, nNode.Vector, h.cfg.Metric),
			})
		}
	}

	// Sort closest first
	for i := 0; i < len(pairs)-1; i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].dist < pairs[i].dist {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	if len(pairs) > m {
		pairs = pairs[:m]
	}
	res := make([]uint64, len(pairs))
	for i := range pairs {
		res[i] = pairs[i].id
	}
	return res
}
