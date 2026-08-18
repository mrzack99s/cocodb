package document

import (
	"fmt"

	"github.com/mrzack99s/cocodb/internal/index"
	"github.com/mrzack99s/cocodb/internal/query"
	"github.com/mrzack99s/cocodb/internal/query/executor"
	"github.com/mrzack99s/cocodb/internal/types"
)

type SortOrder uint8

const (
	Asc SortOrder = iota
	Desc
)

// Query represents a fluent query builder.
type Query struct {
	coll        *Collection
	predicates  []query.Expression
	sortFields  []executor.SortField
	limitVal    int
	offsetVal   int
	vectorField string
	queryVector []float32
	topK        int
}

// Query returns a new Query builder for the collection.
func (c *Collection) Query() *Query {
	return &Query{
		coll: c,
	}
}

// WhereClause provides builder methods for a single field predicate.
type WhereClause struct {
	q     *Query
	field string
}

func (q *Query) Where(field string) *WhereClause {
	return &WhereClause{q: q, field: field}
}

func (w *WhereClause) Eq(val any) *Query {
	w.q.predicates = append(w.q.predicates, query.Eq(w.field, val))
	return w.q
}

func (w *WhereClause) Ne(val any) *Query {
	w.q.predicates = append(w.q.predicates, query.Ne(w.field, val))
	return w.q
}

func (w *WhereClause) Gt(val any) *Query {
	w.q.predicates = append(w.q.predicates, query.Gt(w.field, val))
	return w.q
}

func (w *WhereClause) Gte(val any) *Query {
	w.q.predicates = append(w.q.predicates, query.Gte(w.field, val))
	return w.q
}

func (w *WhereClause) Lt(val any) *Query {
	w.q.predicates = append(w.q.predicates, query.Lt(w.field, val))
	return w.q
}

func (w *WhereClause) Lte(val any) *Query {
	w.q.predicates = append(w.q.predicates, query.Lte(w.field, val))
	return w.q
}

func (w *WhereClause) Between(low, high any) *Query {
	w.q.predicates = append(w.q.predicates, query.Between(w.field, low, high))
	return w.q
}

func (w *WhereClause) In(vals ...any) *Query {
	w.q.predicates = append(w.q.predicates, query.In(w.field, vals...))
	return w.q
}

func (w *WhereClause) Contains(substr string) *Query {
	w.q.predicates = append(w.q.predicates, query.Contains(w.field, substr))
	return w.q
}

func (w *WhereClause) StartsWith(prefix string) *Query {
	w.q.predicates = append(w.q.predicates, query.StartsWith(w.field, prefix))
	return w.q
}

// OrderBy adds a sorting field.
func (q *Query) OrderBy(field string, order SortOrder) *Query {
	q.sortFields = append(q.sortFields, executor.SortField{
		Field: field,
		Desc:  order == Desc,
	})
	return q
}

// Limit sets result limit.
func (q *Query) Limit(limit int) *Query {
	q.limitVal = limit
	return q
}

// Offset sets result offset.
func (q *Query) Offset(offset int) *Query {
	q.offsetVal = offset
	return q
}

// VectorNear attaches a vector similarity search clause.
func (q *Query) VectorNear(field string, vec []float32) *Query {
	q.vectorField = field
	q.queryVector = vec
	return q
}

// TopK sets vector / text Top-K limit.
func (q *Query) TopK(k int) *Query {
	q.topK = k
	q.limitVal = k
	return q
}

// Plan constructs the physical execution pipeline.
func (q *Query) Plan() (executor.Operator, string, error) {
	var op executor.Operator
	var planDesc string

	// Check if any populated secondary index can be used
	var usedIndex *index.SecondaryIndex
	for _, sIdx := range q.coll.secIndexes {
		if sIdx != nil && sIdx.Tree() != nil && sIdx.Tree().Root() != types.InvalidPageID {
			usedIndex = sIdx
			break
		}
	}

	if usedIndex != nil {
		op = executor.NewIndexScan(
			usedIndex.Tree(),
			q.coll.store,
			q.coll.dict,
			q.coll.tx,
			usedIndex.Definition().Unique,
			nil,
		)
		planDesc = fmt.Sprintf("IndexScan(index=%s)", usedIndex.Definition().Name)
	} else {
		op = executor.NewCollectionScan(
			q.coll.primaryIndex.Tree(),
			q.coll.store,
			q.coll.dict,
			q.coll.tx,
		)
		planDesc = fmt.Sprintf("CollectionScan(%s)", q.coll.name)
	}

	// Add filter if predicates exist
	if len(q.predicates) > 0 {
		var combined query.Expression
		if len(q.predicates) == 1 {
			combined = q.predicates[0]
		} else {
			combined = query.And(q.predicates...)
		}
		op = executor.NewFilter(op, combined)
		planDesc = fmt.Sprintf("Filter -> %s", planDesc)
	}

	// Add Sort if requested
	if len(q.sortFields) > 0 {
		op = executor.NewSort(op, q.sortFields)
		planDesc = fmt.Sprintf("Sort -> %s", planDesc)
	}

	// Add Limit / Offset
	if q.limitVal > 0 || q.offsetVal > 0 {
		op = executor.NewLimit(op, q.limitVal, q.offsetVal)
		planDesc = fmt.Sprintf("Limit(%d, offset=%d) -> %s", q.limitVal, q.offsetVal, planDesc)
	}

	return op, planDesc, nil
}

// Explain returns a human-readable query execution plan.
func (q *Query) Explain() (string, error) {
	_, planDesc, err := q.Plan()
	if err != nil {
		return "", err
	}
	return planDesc, nil
}

// All executes the query and returns all matching documents.
func (q *Query) All() ([]Document, error) {
	op, _, err := q.Plan()
	if err != nil {
		return nil, err
	}
	defer op.Close()

	var results []Document
	if q.limitVal > 0 {
		results = make([]Document, 0, q.limitVal)
	}
	for op.Next() {
		view := op.View()
		results = append(results, Document(view.ToMap()))
	}
	return results, op.Err()
}

// First returns the first matching document or ErrDocNotFound.
func (q *Query) First() (Document, error) {
	q.limitVal = 1
	results, err := q.All()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrDocNotFound
	}
	return results[0], nil
}

// Count returns the number of matching documents.
func (q *Query) Count() (int64, error) {
	op, _, err := q.Plan()
	if err != nil {
		return 0, err
	}
	defer op.Close()

	var count int64
	for op.Next() {
		count++
	}
	return count, op.Err()
}
