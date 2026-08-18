package cocodb

import (
	"time"

	"cocodb/document"
	"cocodb/internal/index"
	"cocodb/internal/query"
	"cocodb/internal/vector"
	"cocodb/kv"
	"cocodb/pubsub"
	"cocodb/queue"
)

// Document type alias for public API convenience.
type Document = document.Document

// Schema type alias.
type Schema = document.Schema

// Queue and PubSub aliases
type Queue = queue.Queue
type QueueMessage = queue.Message
type PubSub = pubsub.PubSub
type PubSubMessage = pubsub.Message
type Subscription = pubsub.Subscription

// Queue option helpers
var (
	WithVisibilityTimeout = queue.WithVisibilityTimeout
	WithMaxRetries        = queue.WithMaxRetries
	WithDedupID           = queue.WithDedupID
	WithPriority          = queue.WithPriority
	WithDelay             = queue.WithDelay
	WithQueueTTL          = queue.WithTTL
	WithDLQ               = queue.WithDLQ
)

// PubSub option helpers
var (
	WithConsumerGroup = pubsub.WithConsumerGroup
	WithBufferSize    = pubsub.WithBufferSize
	WithBackpressure  = pubsub.WithBackpressure
	WithPubDedupID    = pubsub.WithDedupID
)

// SortOrder aliases
const (
	Asc  = document.Asc
	Desc = document.Desc
)

// Metric aliases for vector search
const (
	Cosine     = vector.Cosine
	L2         = vector.L2
	DotProduct = vector.DotProduct
)

// TTL option helper for Key/Value
func TTL(d time.Duration) kv.Option {
	return kv.WithTTL(d)
}

// Update operator helpers
var (
	Set       = document.Set
	Unset     = document.Unset
	Increment = document.Increment
	Push      = document.Push
	Remove    = document.Remove
)

// Query expression helpers
var (
	Eq         = query.Eq
	Ne         = query.Ne
	Gt         = query.Gt
	Gte        = query.Gte
	Lt         = query.Lt
	Lte        = query.Lte
	Between    = query.Between
	In         = query.In
	And        = query.And
	Or         = query.Or
	Not        = query.Not
	Contains   = query.Contains
	StartsWith = query.StartsWith
)

// IndexBuilder helps construct secondary index definitions fluently.
type IndexBuilder struct {
	def index.IndexDefinition
}

func Index(fields ...string) *IndexBuilder {
	return &IndexBuilder{
		def: index.IndexDefinition{
			Fields: fields,
		},
	}
}

func (ib *IndexBuilder) On(fields ...string) *IndexBuilder {
	ib.def.Fields = fields
	return ib
}

func (ib *IndexBuilder) Name(name string) *IndexBuilder {
	ib.def.Name = name
	return ib
}

func (ib *IndexBuilder) Unique() *IndexBuilder {
	ib.def.Unique = true
	return ib
}

func (ib *IndexBuilder) Sparse() *IndexBuilder {
	ib.def.Sparse = true
	return ib
}

func (ib *IndexBuilder) Where(expr query.Expression) *IndexBuilder {
	// Partial index filter
	return ib
}

func (ib *IndexBuilder) Build() index.IndexDefinition {
	if ib.def.Name == "" && len(ib.def.Fields) > 0 {
		ib.def.Name = "idx_" + ib.def.Fields[0]
	}
	return ib.def
}

// VectorIndexBuilder helps configure a Vector index.
type VectorIndexBuilder struct {
	field       string
	dims        int
	metric      vector.Metric
	m           int
	efConstruct int
}

func VectorIndex(field string) *VectorIndexBuilder {
	return &VectorIndexBuilder{
		field:  field,
		metric: vector.Cosine,
	}
}

func (vb *VectorIndexBuilder) Dimensions(dims int) *VectorIndexBuilder {
	vb.dims = dims
	return vb
}

func (vb *VectorIndexBuilder) Metric(m vector.Metric) *VectorIndexBuilder {
	vb.metric = m
	return vb
}

func (vb *VectorIndexBuilder) HNSW() *VectorIndexBuilder {
	return vb
}

func (vb *VectorIndexBuilder) M(m int) *VectorIndexBuilder {
	vb.m = m
	return vb
}

func (vb *VectorIndexBuilder) EfConstruction(ef int) *VectorIndexBuilder {
	vb.efConstruct = ef
	return vb
}
