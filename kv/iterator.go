package kv

import (
	"bytes"

	"cocodb/internal/btree"
)

// Iterator provides iteration over Key/Value pairs in a Bucket.
type Iterator struct {
	cursor  *btree.Cursor
	prefix  []byte
	start   []byte
	end     []byte
	reverse bool
	closed  bool
}

// NewIterator creates an iterator wrapping a BTree cursor.
func NewIterator(cursor *btree.Cursor, prefix, start, end []byte, reverse bool) *Iterator {
	it := &Iterator{
		cursor:  cursor,
		prefix:  prefix,
		start:   start,
		end:     end,
		reverse: reverse,
	}
	it.init()
	return it
}

func (it *Iterator) init() {
	if it.reverse {
		if it.end != nil {
			it.cursor.Seek(it.end)
			it.cursor.Prev()
		} else {
			it.cursor.Last()
		}
	} else {
		if it.start != nil {
			it.cursor.Seek(it.start)
		} else if it.prefix != nil {
			it.cursor.Seek(it.prefix)
		} else {
			it.cursor.First()
		}
	}
}

// Valid returns true if pointing to a valid key/value pair.
func (it *Iterator) Valid() bool {
	if it.closed || !it.cursor.Valid() {
		return false
	}
	k := it.cursor.Key()
	if it.prefix != nil && !bytes.HasPrefix(k, it.prefix) {
		return false
	}
	if it.end != nil && bytes.Compare(k, it.end) >= 0 {
		return false
	}
	if it.start != nil && bytes.Compare(k, it.start) < 0 {
		return false
	}
	return true
}

// Next advances the iterator.
func (it *Iterator) Next() bool {
	if it.closed {
		return false
	}
	if it.reverse {
		_ = it.cursor.Prev()
	} else {
		_ = it.cursor.Next()
	}
	return it.Valid()
}

// Key returns the current key copy.
func (it *Iterator) Key() []byte {
	return it.cursor.Key()
}

// Value returns the current value copy.
func (it *Iterator) Value() []byte {
	return it.cursor.Value()
}

// Err returns any cursor error.
func (it *Iterator) Err() error {
	return it.cursor.Err()
}

// Close closes the iterator.
func (it *Iterator) Close() error {
	it.closed = true
	return it.cursor.Close()
}
