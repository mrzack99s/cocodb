package document

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/cson"
	"github.com/mrzack99s/cocodb/internal/index"
	"github.com/mrzack99s/cocodb/internal/record"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
)

var (
	ErrDocNotFound = errors.New("coco/document: document not found")
)

// IndexDefiner is implemented by index builder types.
type IndexDefiner interface {
	Build() index.IndexDefinition
}

// Collection manages NoSQL document operations and indexes on top of the storage kernel.
type Collection struct {
	mu           sync.RWMutex
	name         string
	id           types.ObjectID
	pager        storage.Pager
	tx           *txn.Transaction
	store        *record.Store
	dict         *cson.FieldDictionary
	primaryIndex *index.PrimaryIndex
	secIndexes   map[string]*index.SecondaryIndex
	schema       Schema
}

// NewCollection opens or wraps a Collection.
func NewCollection(
	name string,
	id types.ObjectID,
	pager storage.Pager,
	tx *txn.Transaction,
	store *record.Store,
	dict *cson.FieldDictionary,
	primaryRoot types.PageID,
) *Collection {
	if dict == nil {
		dict = cson.NewFieldDictionary()
	}
	return &Collection{
		name:         name,
		id:           id,
		pager:        pager,
		tx:           tx,
		store:        store,
		dict:         dict,
		primaryIndex: index.NewPrimaryIndex(pager, primaryRoot),
		secIndexes:   make(map[string]*index.SecondaryIndex),
	}
}

func (c *Collection) Name() string {
	return c.name
}

func (c *Collection) PrimaryRoot() types.PageID {
	return c.primaryIndex.Root()
}

func (c *Collection) Store() *record.Store {
	return c.store
}

func (c *Collection) Dictionary() *cson.FieldDictionary {
	return c.dict
}

func (c *Collection) PrimaryIndex() *index.PrimaryIndex {
	return c.primaryIndex
}

func (c *Collection) SecondaryIndexes() map[string]*index.SecondaryIndex {
	return c.secIndexes
}

// SetSchema applies an optional schema validator.
func (c *Collection) SetSchema(schema Schema) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.schema = schema
}

// AddSecondaryIndex registers a secondary index.
func (c *Collection) AddSecondaryIndex(idx *index.SecondaryIndex) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.secIndexes[idx.Definition().Name] = idx
}

// CreateIndex creates and registers a secondary index on the collection.
func (c *Collection) CreateIndex(builder any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var def index.IndexDefinition
	switch b := builder.(type) {
	case index.IndexDefinition:
		def = b
	case IndexDefiner:
		def = b.Build()
	default:
		return fmt.Errorf("coco/document: unsupported index definition type %T", builder)
	}

	if def.Name == "" && len(def.Fields) > 0 {
		def.Name = "idx_" + def.Fields[0]
	}

	sIdx := index.NewSecondaryIndex(def, c.pager, types.InvalidPageID)
	c.secIndexes[def.Name] = sIdx
	return nil
}

// Insert adds a new document into the collection.
func (c *Collection) Insert(doc Document) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check / assign _id
	idVal, ok := doc["_id"]
	var id string
	if !ok || idVal == nil {
		id = generateUUID()
		doc["_id"] = id
	} else {
		id = fmt.Sprintf("%v", idVal)
	}

	// Validate schema if configured
	if c.schema != nil {
		if err := c.schema.Validate(doc); err != nil {
			return "", err
		}
	}

	// Encode to CSON
	csonBytes, err := cson.Encode(doc, c.dict)
	if err != nil {
		return "", err
	}

	// Write record
	recID, err := c.store.WriteRecord(c.tx, csonBytes, types.InvalidRecordID)
	if err != nil {
		return "", err
	}

	// Update primary index
	if err := c.primaryIndex.Put(id, recID); err != nil {
		return "", err
	}

	// Update secondary indexes only if defined
	if len(c.secIndexes) > 0 {
		view, err := cson.NewDocumentView(csonBytes, c.dict)
		if err != nil {
			_ = c.primaryIndex.Delete(id)
			return "", err
		}

		for _, sIdx := range c.secIndexes {
			if err := sIdx.Insert(view, recID); err != nil {
				_ = c.primaryIndex.Delete(id)
				return "", err
			}
		}
	}

	return id, nil
}

// InsertMany inserts multiple documents.
func (c *Collection) InsertMany(docs ...Document) ([]string, error) {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		id, err := c.Insert(doc)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetView returns a zero-allocation DocumentView for the document.
func (c *Collection) GetView(id string) (*cson.DocumentView, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	recID, found, err := c.primaryIndex.Get(id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrDocNotFound
	}

	_, payload, err := c.store.ReadRecord(c.tx, recID)
	if err != nil {
		return nil, ErrDocNotFound
	}

	return cson.NewDocumentView(payload, c.dict)
}

// Get retrieves the full document by _id.
func (c *Collection) Get(id string) (Document, error) {
	view, err := c.GetView(id)
	if err != nil {
		return nil, err
	}
	return Document(view.ToMap()), nil
}

// Replace completely replaces an existing document.
func (c *Collection) Replace(id string, newDoc Document) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	recID, found, err := c.primaryIndex.Get(id)
	if err != nil {
		return err
	}
	if !found {
		return ErrDocNotFound
	}

	newDoc["_id"] = id
	if c.schema != nil {
		if err := c.schema.Validate(newDoc); err != nil {
			return err
		}
	}

	// Read old view to remove from secondary indexes
	_, oldPayload, err := c.store.ReadRecord(c.tx, recID)
	if err == nil {
		if oldView, err := cson.NewDocumentView(oldPayload, c.dict); err == nil {
			for _, sIdx := range c.secIndexes {
				_ = sIdx.Delete(oldView, recID)
			}
		}
	}

	// Write new CSON version
	csonBytes, err := cson.Encode(newDoc, c.dict)
	if err != nil {
		return err
	}

	newRecID, err := c.store.WriteRecord(c.tx, csonBytes, recID)
	if err != nil {
		return err
	}

	// Update primary index
	if err := c.primaryIndex.Put(id, newRecID); err != nil {
		return err
	}

	// Insert into secondary indexes
	if newView, err := cson.NewDocumentView(csonBytes, c.dict); err == nil {
		for _, sIdx := range c.secIndexes {
			_ = sIdx.Insert(newView, newRecID)
		}
	}

	return nil
}

// Update applies partial updates to an existing document.
func (c *Collection) Update(id string, ops ...UpdateOp) error {
	doc, err := c.Get(id)
	if err != nil {
		return err
	}

	for _, op := range ops {
		switch op.Type {
		case OpSet:
			doc[op.Field] = op.Value
		case OpUnset:
			delete(doc, op.Field)
		case OpIncrement:
			var current int64
			if existing, ok := doc[op.Field]; ok {
				switch n := existing.(type) {
				case int:
					current = int64(n)
				case int64:
					current = n
				case float64:
					current = int64(n)
				}
			}
			doc[op.Field] = current + op.Value.(int64)
		case OpPush:
			var arr []any
			if existing, ok := doc[op.Field]; ok {
				if a, ok := existing.([]any); ok {
					arr = a
				}
			}
			arr = append(arr, op.Value)
			doc[op.Field] = arr
		}
	}

	return c.Replace(id, doc)
}

// Delete removes a document by _id.
func (c *Collection) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	recID, found, err := c.primaryIndex.Get(id)
	if err != nil {
		return err
	}
	if !found {
		return ErrDocNotFound
	}

	// Delete from secondary indexes
	_, oldPayload, err := c.store.ReadRecord(c.tx, recID)
	if err == nil {
		if oldView, err := cson.NewDocumentView(oldPayload, c.dict); err == nil {
			for _, sIdx := range c.secIndexes {
				_ = sIdx.Delete(oldView, recID)
			}
		}
	}

	if err := c.primaryIndex.Delete(id); err != nil {
		return err
	}

	return c.store.DeleteRecord(c.tx, recID)
}

// Count returns the total number of documents in the collection.
func (c *Collection) Count() (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cur := btree.NewCursor(c.primaryIndex.Tree())
	defer cur.Close()

	if !cur.First() {
		return 0, cur.Err()
	}

	var count int64
	for cur.Valid() {
		count++
		if !cur.Next() {
			break
		}
	}
	return count, cur.Err()
}

var docIDSeq atomic.Uint64
var docIDPrefix = uint64(time.Now().UnixNano())

func generateUUID() string {
	seq := docIDSeq.Add(1)
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[0:8], docIDPrefix)
	binary.BigEndian.PutUint64(buf[8:16], seq)
	return hex.EncodeToString(buf[:])
}
