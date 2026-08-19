package cocodb

import (
	"fmt"

	"github.com/mrzack99s/cocodb/document"
	"github.com/mrzack99s/cocodb/internal/catalog"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
	"github.com/mrzack99s/cocodb/kv"
)

// Tx represents a user-facing transactional boundary.
type Tx struct {
	db          *DB
	internalTx  *txn.Transaction
	buckets     map[string]*kv.Bucket
	collections map[string]*document.Collection
	childTxs    map[*DB]*Tx
}

func (tx *Tx) childTx(target *DB) (*Tx, error) {
	if target == tx.db {
		return tx, nil
	}
	if child, ok := tx.childTxs[target]; ok {
		return child, nil
	}
	child, err := target.Begin(tx.ReadOnly())
	if err != nil {
		return nil, err
	}
	if tx.childTxs == nil {
		tx.childTxs = make(map[*DB]*Tx)
	}
	tx.childTxs[target] = child
	return child, nil
}

func (tx *Tx) ID() types.TxnID {
	return tx.internalTx.ID()
}

func (tx *Tx) ReadOnly() bool {
	return tx.internalTx.ReadOnly()
}

// Bucket returns a transactional Bucket handle.
func (tx *Tx) Bucket(name string) *kv.Bucket {
	if target := tx.db.modelDB(ModelKV); target != tx.db {
		child, err := tx.childTx(target)
		if err != nil {
			return nil
		}
		return child.Bucket(name)
	}
	if tx.buckets == nil {
		tx.buckets = make(map[string]*kv.Bucket)
	} else if b, ok := tx.buckets[name]; ok {
		return b
	}

	obj, ok := tx.db.catalog.GetObject(catalog.ObjectBucket, name)
	var root types.PageID = types.InvalidPageID
	var objID types.ObjectID = types.InvalidObjectID

	if ok {
		root = obj.Root
		objID = obj.ID
	} else if !tx.ReadOnly() {
		// Auto-create bucket
		obj = &catalog.Object{
			Type: catalog.ObjectBucket,
			Name: name,
			Root: types.InvalidPageID,
		}
		_ = tx.db.catalog.PutObject(obj)
		root = obj.Root
		objID = obj.ID
	}

	tree := tx.db.getOrOpenTree(root)
	b := kv.NewBucket(name, objID, tree, tx.internalTx, tx.db.ttlIndex)
	tx.buckets[name] = b
	return b
}

// Collection returns a transactional Collection handle.
func (tx *Tx) Collection(name string) *document.Collection {
	if target := tx.db.modelDB(ModelDocument); target != tx.db {
		child, err := tx.childTx(target)
		if err != nil {
			return nil
		}
		return child.Collection(name)
	}
	if tx.collections == nil {
		tx.collections = make(map[string]*document.Collection)
	} else if c, ok := tx.collections[name]; ok {
		return c
	}

	obj, ok := tx.db.catalog.GetObject(catalog.ObjectCollection, name)
	var root types.PageID = types.InvalidPageID
	var objID types.ObjectID = types.InvalidObjectID
	dict := tx.db.getOrOpenDict(name)

	if ok {
		root = obj.Root
		objID = obj.ID
	} else if !tx.ReadOnly() {
		obj = &catalog.Object{
			Type:      catalog.ObjectCollection,
			Name:      name,
			Root:      types.InvalidPageID,
			ExtraData: dict.Encode(),
		}
		_ = tx.db.catalog.PutObject(obj)
		root = obj.Root
		objID = obj.ID
	}

	coll := document.NewCollection(name, objID, tx.db.pager, tx.internalTx, tx.db.store, dict, root)
	tx.collections[name] = coll
	return coll
}

// Savepoint creates a named savepoint.
func (tx *Tx) Savepoint(name string) error {
	return tx.internalTx.Savepoint(name)
}

// RollbackTo rolls back all mutations since the named savepoint.
func (tx *Tx) RollbackTo(name string) error {
	return tx.internalTx.RollbackTo(name)
}

// Commit commits the transaction.
func (tx *Tx) Commit() error {
	// Sync bucket roots back to catalog if created/updated
	for name, b := range tx.buckets {
		if obj, ok := tx.db.catalog.GetObject(catalog.ObjectBucket, name); ok {
			if obj.Root != b.Root() {
				obj.Root = b.Root()
				_ = tx.db.catalog.PutObject(obj)
			}
		}
	}

	// Sync collection primary roots and dictionary back to catalog
	for name, c := range tx.collections {
		if obj, ok := tx.db.catalog.GetObject(catalog.ObjectCollection, name); ok {
			if obj.Root != c.PrimaryRoot() {
				obj.Root = c.PrimaryRoot()
			}
			obj.ExtraData = c.Dictionary().Encode()
			_ = tx.db.catalog.PutObject(obj)
		}
	}

	err := tx.internalTx.Commit()
	if err == nil {
		for _, child := range tx.childTxs {
			if childErr := child.Commit(); childErr != nil {
				err = childErr
				break
			}
		}
	}

	tx.db = nil
	tx.internalTx = nil
	tx.buckets = nil
	tx.collections = nil
	tx.childTxs = nil
	txPool.Put(tx)

	if err != nil {
		return fmt.Errorf("transaction commit failed: %w", err)
	}
	return nil
}

// Rollback rolls back the transaction.
func (tx *Tx) Rollback() error {
	if tx.internalTx == nil {
		return nil
	}
	err := tx.internalTx.Rollback()
	for _, child := range tx.childTxs {
		_ = child.Rollback()
	}

	tx.db = nil
	tx.internalTx = nil
	tx.buckets = nil
	tx.collections = nil
	tx.childTxs = nil
	txPool.Put(tx)

	return err
}
