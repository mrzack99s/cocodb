package vector

import (
	internalVec "github.com/mrzack99s/cocodb/internal/vector"
)

type Metric = internalVec.Metric

const (
	Cosine     = internalVec.Cosine
	L2         = internalVec.L2
	DotProduct = internalVec.DotProduct
)

type Match = internalVec.Match
type HNSW = internalVec.HNSW
type HNSWConfig = internalVec.HNSWConfig

func NewHNSW(cfg HNSWConfig) *HNSW {
	return internalVec.NewHNSW(cfg)
}

func DefaultHNSWConfig(dims int) HNSWConfig {
	return internalVec.DefaultHNSWConfig(dims)
}
