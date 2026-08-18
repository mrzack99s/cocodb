package index

import (
	"encoding/binary"
	"fmt"
	"time"

	"cocodb/internal/btree"
	"cocodb/internal/cson"
	"cocodb/internal/storage"
	"cocodb/internal/types"
)

// IndexDefinition configures a secondary index.
type IndexDefinition struct {
	Name         string
	Fields       []string
	Unique       bool
	Sparse       bool
	PartialField string
	PartialVal   any
}

// SecondaryIndex manages a single secondary index backed by a B+Tree.
type SecondaryIndex struct {
	def   IndexDefinition
	tree  *btree.BTree
	pager storage.Pager
}

func NewSecondaryIndex(def IndexDefinition, pager storage.Pager, root types.PageID) *SecondaryIndex {
	return &SecondaryIndex{
		def:   def,
		tree:  btree.NewBTree(pager, root),
		pager: pager,
	}
}

func (si *SecondaryIndex) Definition() IndexDefinition {
	return si.def
}

func (si *SecondaryIndex) Root() types.PageID {
	return si.tree.Root()
}

func (si *SecondaryIndex) Tree() *btree.BTree {
	return si.tree
}

// BuildKey constructs the binary index key from a DocumentView.
// Returns (key, shouldIndex).
func (si *SecondaryIndex) BuildKey(view *cson.DocumentView, recID types.RecordID) ([]byte, bool) {
	// Check partial filter if configured
	if si.def.PartialField != "" {
		val, ok := view.Get(si.def.PartialField)
		if !ok || val != si.def.PartialVal {
			return nil, false
		}
	}

	var encodedComponents [][]byte
	for _, field := range si.def.Fields {
		val, ok := view.Get(field)
		if !ok {
			if si.def.Sparse {
				return nil, false
			}
			encodedComponents = append(encodedComponents, btree.EncodeNull())
			continue
		}

		enc := encodeIndexValue(val)
		encodedComponents = append(encodedComponents, enc)
	}

	composite := btree.EncodeComposite(encodedComponents...)
	if !si.def.Unique {
		composite = btree.AppendRecordID(composite, recID)
	}

	return composite, true
}

func encodeIndexValue(v any) []byte {
	if v == nil {
		return btree.EncodeNull()
	}
	switch val := v.(type) {
	case bool:
		return btree.EncodeBool(val)
	case int:
		return btree.EncodeInt64(int64(val))
	case int64:
		return btree.EncodeInt64(val)
	case int32:
		return btree.EncodeInt64(int64(val))
	case uint:
		return btree.EncodeUint64(uint64(val))
	case uint64:
		return btree.EncodeUint64(val)
	case uint32:
		return btree.EncodeUint64(uint64(val))
	case float64:
		return btree.EncodeFloat64(val)
	case float32:
		return btree.EncodeFloat64(float64(val))
	case string:
		return btree.EncodeString(val)
	case []byte:
		return btree.EncodeBytes(val)
	case time.Time:
		return btree.EncodeTime(val)
	default:
		return btree.EncodeString(fmt.Sprintf("%v", v))
	}
}

// Insert adds a document entry into the index.
func (si *SecondaryIndex) Insert(view *cson.DocumentView, recID types.RecordID) error {
	k, shouldIndex := si.BuildKey(view, recID)
	if !shouldIndex {
		return nil
	}

	if si.def.Unique {
		existingVal, found, err := si.tree.Search(k)
		if err != nil {
			return err
		}
		if found && len(existingVal) >= 8 {
			existingRecID := types.RecordID(binary.BigEndian.Uint64(existingVal))
			if existingRecID != recID {
				return fmt.Errorf("unique index violation on index %q for key", si.def.Name)
			}
		}
		val := make([]byte, 8)
		binary.BigEndian.PutUint64(val, uint64(recID))
		return si.tree.Insert(k, val)
	}

	// Non-unique index: RecordID is embedded in key
	return si.tree.Insert(k, []byte{1})
}

// Delete removes a document entry from the index.
func (si *SecondaryIndex) Delete(view *cson.DocumentView, recID types.RecordID) error {
	k, shouldIndex := si.BuildKey(view, recID)
	if !shouldIndex {
		return nil
	}
	return si.tree.Delete(k)
}
