package kv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"

	"cocodb/internal/btree"
	"cocodb/internal/index"
	"cocodb/internal/txn"
	"cocodb/internal/types"
)

var (
	ErrNotFound = errors.New("coco/kv: key not found")
	ErrConflict = errors.New("coco/kv: compare-and-swap conflict")
)

type Option func(*PutOptions)

type PutOptions struct {
	TTL time.Duration
}

func WithTTL(d time.Duration) Option {
	return func(o *PutOptions) {
		o.TTL = d
	}
}

// Bucket provides an ordered Key/Value store interface backed by a transactional B+Tree.
type Bucket struct {
	name     string
	id       types.ObjectID
	tree     *btree.BTree
	tx       *txn.Transaction
	ttlIndex *index.TTLIndex
}

// NewBucket creates a Bucket handle.
func NewBucket(name string, id types.ObjectID, tree *btree.BTree, tx *txn.Transaction, ttlIndex *index.TTLIndex) *Bucket {
	return &Bucket{
		name:     name,
		id:       id,
		tree:     tree,
		tx:       tx,
		ttlIndex: ttlIndex,
	}
}

// Name returns the bucket name.
func (b *Bucket) Name() string {
	return b.name
}

// ID returns the bucket's internal ObjectID.
func (b *Bucket) ID() types.ObjectID {
	return b.id
}

// Root returns the current BTree root page ID.
func (b *Bucket) Root() types.PageID {
	return b.tree.Root()
}

// Put inserts or updates a key-value pair.
func (b *Bucket) Put(key, value []byte, opts ...Option) error {
	var opt PutOptions
	for _, o := range opts {
		o(&opt)
	}

	if err := b.tree.Insert(key, value); err != nil {
		return err
	}

	if opt.TTL > 0 && b.ttlIndex != nil {
		expireAt := time.Now().Add(opt.TTL)
		_ = b.ttlIndex.Add(expireAt, b.id, key)
	}

	return nil
}

// Get retrieves the value for a key.
func (b *Bucket) Get(key []byte) ([]byte, error) {
	val, found, err := b.tree.Search(key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return val, nil
}

// Exists checks if a key exists in the bucket.
func (b *Bucket) Exists(key []byte) (bool, error) {
	_, found, err := b.tree.Search(key)
	return found, err
}

// Delete removes a key from the bucket.
func (b *Bucket) Delete(key []byte) error {
	return b.tree.Delete(key)
}

// PutIfAbsent inserts value only if key does not already exist. Returns true if inserted.
func (b *Bucket) PutIfAbsent(key, value []byte, opts ...Option) (bool, error) {
	_, found, err := b.tree.Search(key)
	if err != nil {
		return false, err
	}
	if found {
		return false, nil
	}
	err = b.Put(key, value, opts...)
	return err == nil, err
}

// CompareAndSwap atomically replaces expected value with replacement. Returns true if swapped.
func (b *Bucket) CompareAndSwap(key, expected, replacement []byte, opts ...Option) (bool, error) {
	curVal, found, err := b.tree.Search(key)
	if err != nil {
		return false, err
	}
	if !found {
		if expected == nil {
			err = b.Put(key, replacement, opts...)
			return err == nil, err
		}
		return false, nil
	}
	if !bytes.Equal(curVal, expected) {
		return false, nil
	}
	err = b.Put(key, replacement, opts...)
	return err == nil, err
}

// Increment atomically adds delta to an int64 value stored at key.
func (b *Bucket) Increment(key []byte, delta int64) (int64, error) {
	val, found, err := b.tree.Search(key)
	if err != nil {
		return 0, err
	}

	var current int64
	if found {
		if len(val) == 8 {
			current = int64(binary.BigEndian.Uint64(val))
		}
	}

	newVal := current + delta
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(newVal))

	if err := b.Put(key, buf[:]); err != nil {
		return 0, err
	}
	return newVal, nil
}

// Decrement atomically subtracts delta from an int64 value stored at key.
func (b *Bucket) Decrement(key []byte, delta int64) (int64, error) {
	return b.Increment(key, -delta)
}

// Batch applies a batch of Put and Delete operations atomically.
func (b *Bucket) Batch(batch *Batch) error {
	for _, op := range batch.Ops() {
		switch op.Type {
		case BatchPut:
			if err := b.Put(op.Key, op.Value); err != nil {
				return err
			}
		case BatchDelete:
			if err := b.Delete(op.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

// Range returns an iterator over keys in [start, end).
func (b *Bucket) Range(start, end []byte) *Iterator {
	cur := btree.NewCursor(b.tree)
	return NewIterator(cur, nil, start, end, false)
}

// Prefix returns an iterator over keys matching prefix.
func (b *Bucket) Prefix(prefix []byte) *Iterator {
	cur := btree.NewCursor(b.tree)
	return NewIterator(cur, prefix, nil, nil, false)
}

// Iterator returns a forward iterator across the entire bucket.
func (b *Bucket) Iterator() *Iterator {
	cur := btree.NewCursor(b.tree)
	return NewIterator(cur, nil, nil, nil, false)
}

// ReverseIterator returns a reverse iterator across the entire bucket.
func (b *Bucket) ReverseIterator() *Iterator {
	cur := btree.NewCursor(b.tree)
	return NewIterator(cur, nil, nil, nil, true)
}
