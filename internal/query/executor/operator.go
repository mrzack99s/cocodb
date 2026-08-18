package executor

import (
	"encoding/binary"
	"sort"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/cson"
	"github.com/mrzack99s/cocodb/internal/query"
	"github.com/mrzack99s/cocodb/internal/record"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
)

// Operator defines the Volcano execution iterator model.
type Operator interface {
	Next() bool
	View() *cson.DocumentView
	RecordID() types.RecordID
	Err() error
	Close() error
}

// CollectionScan scans all documents in a collection via the primary index.
type CollectionScan struct {
	cursor  *btree.Cursor
	store   *record.Store
	dict    *cson.FieldDictionary
	tx      *txn.Transaction
	curView cson.DocumentView
	hasView bool
	curRec  types.RecordID
	err     error
	started bool
}

func NewCollectionScan(primaryTree *btree.BTree, store *record.Store, dict *cson.FieldDictionary, tx *txn.Transaction) *CollectionScan {
	return &CollectionScan{
		cursor: btree.NewCursor(primaryTree),
		store:  store,
		dict:   dict,
		tx:     tx,
	}
}

func (cs *CollectionScan) Next() bool {
	for {
		if !cs.started {
			cs.started = true
			if !cs.cursor.First() {
				return false
			}
		} else {
			if !cs.cursor.Next() {
				return false
			}
		}

		val := cs.cursor.Value()
		if len(val) < 8 {
			continue
		}
		recID := types.RecordID(binary.BigEndian.Uint64(val))

		_, payload, err := cs.store.ReadRecordDirect(cs.tx, recID)
		if err != nil {
			continue
		}

		if err := cs.curView.Reset(payload, cs.dict); err != nil {
			continue
		}

		cs.hasView = true
		cs.curRec = recID
		return true
	}
}

func (cs *CollectionScan) View() *cson.DocumentView {
	if !cs.hasView {
		return nil
	}
	return &cs.curView
}

func (cs *CollectionScan) RecordID() types.RecordID {
	return cs.curRec
}

func (cs *CollectionScan) Err() error {
	if cs.err != nil {
		return cs.err
	}
	return cs.cursor.Err()
}

func (cs *CollectionScan) Close() error {
	return cs.cursor.Close()
}

// IndexScan scans a secondary index.
type IndexScan struct {
	cursor  *btree.Cursor
	store   *record.Store
	dict    *cson.FieldDictionary
	tx      *txn.Transaction
	isUniq  bool
	prefix  []byte
	curView cson.DocumentView
	hasView bool
	curRec  types.RecordID
	err     error
	started bool
}

func NewIndexScan(secTree *btree.BTree, store *record.Store, dict *cson.FieldDictionary, tx *txn.Transaction, isUnique bool, prefix []byte) *IndexScan {
	return &IndexScan{
		cursor: btree.NewCursor(secTree),
		store:  store,
		dict:   dict,
		tx:     tx,
		isUniq: isUnique,
		prefix: prefix,
	}
}

func (is *IndexScan) Next() bool {
	for {
		if !is.started {
			is.started = true
			if is.prefix != nil {
				if !is.cursor.Seek(is.prefix) {
					return false
				}
			} else {
				if !is.cursor.First() {
					return false
				}
			}
		} else {
			if !is.cursor.Next() {
				return false
			}
		}

		var recID types.RecordID
		if is.isUniq {
			val := is.cursor.Value()
			if len(val) < 8 {
				continue
			}
			recID = types.RecordID(binary.BigEndian.Uint64(val))
		} else {
			k := is.cursor.Key()
			recID, _ = btree.ExtractRecordID(k)
		}

		_, payload, err := is.store.ReadRecordDirect(is.tx, recID)
		if err != nil {
			continue
		}

		if err := is.curView.Reset(payload, is.dict); err != nil {
			continue
		}

		is.hasView = true
		is.curRec = recID
		return true
	}
}

func (is *IndexScan) View() *cson.DocumentView {
	if !is.hasView {
		return nil
	}
	return &is.curView
}

func (is *IndexScan) RecordID() types.RecordID {
	return is.curRec
}

func (is *IndexScan) Err() error {
	return is.cursor.Err()
}

func (is *IndexScan) Close() error {
	return is.cursor.Close()
}

// Filter evaluates a filter expression on top of a child operator.
type Filter struct {
	child Operator
	expr  query.Expression
}

func NewFilter(child Operator, expr query.Expression) *Filter {
	return &Filter{
		child: child,
		expr:  expr,
	}
}

func (f *Filter) Next() bool {
	for f.child.Next() {
		if f.expr == nil || f.expr.Evaluate(f.child.View()) {
			return true
		}
	}
	return false
}

func (f *Filter) View() *cson.DocumentView {
	return f.child.View()
}

func (f *Filter) RecordID() types.RecordID {
	return f.child.RecordID()
}

func (f *Filter) Err() error {
	return f.child.Err()
}

func (f *Filter) Close() error {
	return f.child.Close()
}

// Limit limits and offsets results.
type Limit struct {
	child   Operator
	limit   int
	offset  int
	seen    int
	yielded int
}

func NewLimit(child Operator, limit, offset int) *Limit {
	return &Limit{
		child:  child,
		limit:  limit,
		offset: offset,
	}
}

func (l *Limit) Next() bool {
	for l.seen < l.offset {
		if !l.child.Next() {
			return false
		}
		l.seen++
	}

	if l.limit > 0 && l.yielded >= l.limit {
		return false
	}

	if l.child.Next() {
		l.yielded++
		return true
	}
	return false
}

func (l *Limit) View() *cson.DocumentView {
	return l.child.View()
}

func (l *Limit) RecordID() types.RecordID {
	return l.child.RecordID()
}

func (l *Limit) Err() error {
	return l.child.Err()
}

func (l *Limit) Close() error {
	return l.child.Close()
}

type SortField struct {
	Field string
	Desc  bool
}

// Sort sorts results in memory.
type Sort struct {
	child        Operator
	sortFields   []SortField
	materialized []item
	idx          int
	started      bool
	err          error
}

type item struct {
	view  cson.DocumentView
	recID types.RecordID
}

func NewSort(child Operator, fields []SortField) *Sort {
	return &Sort{
		child:      child,
		sortFields: fields,
		idx:        -1,
	}
}

func (s *Sort) Next() bool {
	if !s.started {
		s.started = true
		for s.child.Next() {
			s.materialized = append(s.materialized, item{
				view:  *s.child.View(),
				recID: s.child.RecordID(),
			})
		}
		if err := s.child.Err(); err != nil {
			s.err = err
			return false
		}

		sort.Slice(s.materialized, func(i, j int) bool {
			vA := &s.materialized[i].view
			vB := &s.materialized[j].view
			for _, sf := range s.sortFields {
				valA, _ := vA.Get(sf.Field)
				valB, _ := vB.Get(sf.Field)
				cmp := compareAny(valA, valB)
				if cmp != 0 {
					if sf.Desc {
						return cmp > 0
					}
					return cmp < 0
				}
			}
			return false
		})
	}

	s.idx++
	return s.idx < len(s.materialized)
}

func (s *Sort) View() *cson.DocumentView {
	if s.idx >= 0 && s.idx < len(s.materialized) {
		return &s.materialized[s.idx].view
	}
	return nil
}

func (s *Sort) RecordID() types.RecordID {
	if s.idx >= 0 && s.idx < len(s.materialized) {
		return s.materialized[s.idx].recID
	}
	return types.InvalidRecordID
}

func (s *Sort) Err() error {
	return s.err
}

func (s *Sort) Close() error {
	return s.child.Close()
}

func compareAny(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	switch vA := a.(type) {
	case int64:
		if vB, ok := b.(int64); ok {
			if vA < vB {
				return -1
			} else if vA > vB {
				return 1
			}
			return 0
		}
	case float64:
		if vB, ok := b.(float64); ok {
			if vA < vB {
				return -1
			} else if vA > vB {
				return 1
			}
			return 0
		}
	case string:
		if vB, ok := b.(string); ok {
			if vA < vB {
				return -1
			} else if vA > vB {
				return 1
			}
			return 0
		}
	}
	return 0
}
