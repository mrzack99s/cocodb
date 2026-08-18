package record

import (
	"io"

	"cocodb/internal/txn"
	"cocodb/internal/types"
)

// BlobWriter streams large binary objects into the store.
type BlobWriter struct {
	store *Store
	tx    *txn.Transaction
	buf   []byte
	recID types.RecordID
}

// NewBlobWriter creates a streaming blob writer.
func NewBlobWriter(store *Store, tx *txn.Transaction) *BlobWriter {
	return &BlobWriter{
		store: store,
		tx:    tx,
		buf:   make([]byte, 0),
	}
}

func (bw *BlobWriter) Write(p []byte) (int, error) {
	bw.buf = append(bw.buf, p...)
	return len(p), nil
}

// Close finalizes writing the blob and returns the generated RecordID.
func (bw *BlobWriter) Close() (types.RecordID, error) {
	recID, err := bw.store.WriteRecord(bw.tx, bw.buf, types.InvalidRecordID)
	if err != nil {
		return types.InvalidRecordID, err
	}
	bw.recID = recID
	return recID, nil
}

// BlobReader streams large binary objects out of the store.
type BlobReader struct {
	data   []byte
	offset int
}

// OpenBlobReader creates a streaming blob reader for a record.
func OpenBlobReader(store *Store, tx *txn.Transaction, recID types.RecordID) (*BlobReader, error) {
	_, data, err := store.ReadRecord(tx, recID)
	if err != nil {
		return nil, err
	}
	return &BlobReader{
		data: data,
	}, nil
}

func (br *BlobReader) Read(p []byte) (int, error) {
	if br.offset >= len(br.data) {
		return 0, io.EOF
	}
	n := copy(p, br.data[br.offset:])
	br.offset += n
	return n, nil
}
