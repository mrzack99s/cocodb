package record

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
)

const (
	MaxInlinePayload = 4000 // fits comfortably in a 16 KiB slotted page
)

type Store struct {
	pager       storage.Pager
	directory   *Directory
	manager     *txn.TxnManager
	nextID      uint64
	activePages []*storage.Page
}

func NewStore(pager storage.Pager, dir *Directory, tm *txn.TxnManager) *Store {
	return &Store{
		pager:       pager,
		directory:   dir,
		manager:     tm,
		nextID:      1000,
		activePages: make([]*storage.Page, 0),
	}
}

func (s *Store) allocateID() types.RecordID {
	id := atomic.AddUint64(&s.nextID, 1)
	return types.RecordID(id)
}

func (s *Store) WriteRecord(tx *txn.Transaction, payload []byte, recID types.RecordID) (types.RecordID, error) {
	if tx == nil {
		return types.InvalidRecordID, txn.ErrTxnClosed
	}

	var prevPhysID types.RecordID = types.InvalidRecordID
	stableRecID := recID

	if stableRecID != types.InvalidRecordID && stableRecID != 0 {
		// Update existing record: find old location
		oldLoc, found, err := s.directory.Get(stableRecID)
		if err == nil && found {
			oldPage, err := s.pager.Get(oldLoc.Page)
			if err == nil {
				sp := storage.WrapSlotted(oldPage)
				recBytes, err := sp.Get(oldLoc.Slot)
				if err == nil {
					oldHdr, err := DecodeHeader(recBytes)
					if err == nil {
						// Mark old record superseded by current transaction
						oldHdr.EndTxn = tx.ID()
						oldHdr.EncodeTo(recBytes[:RecordHeaderSize])
						_ = tx.TrackDirty(oldPage)

						// Assign physical version ID to old version so previous snapshots can reach it
						prevPhysID = s.allocateID()
						_ = s.directory.Put(prevPhysID, oldLoc)
					}
				}
			}
		}
	} else {
		// Generate new stable record ID
		stableRecID = s.allocateID()
	}

	var recordBytes []byte
	var flags uint16
	if payload == nil {
		flags |= FlagDeleted
	}

	if len(payload) > MaxInlinePayload {
		flags |= FlagOverflow
		firstOverflow, err := s.writeOverflowChain(tx, payload)
		if err != nil {
			return types.InvalidRecordID, err
		}
		ptrBytes := make([]byte, 12)
		binary.BigEndian.PutUint64(ptrBytes[0:8], uint64(firstOverflow))
		binary.BigEndian.PutUint32(ptrBytes[8:12], uint32(len(payload)))

		hdr := Header{
			RecordID:      stableRecID,
			BeginTxn:      tx.ID(),
			EndTxn:        0,
			PrevVersion:   prevPhysID,
			Flags:         flags,
			PayloadLength: uint32(len(payload)),
		}
		recordBytes = make([]byte, RecordHeaderSize+len(ptrBytes))
		hdr.EncodeTo(recordBytes[:RecordHeaderSize])
		copy(recordBytes[RecordHeaderSize:], ptrBytes)
	} else {
		hdr := Header{
			RecordID:      stableRecID,
			BeginTxn:      tx.ID(),
			EndTxn:        0,
			PrevVersion:   prevPhysID,
			Flags:         flags,
			PayloadLength: uint32(len(payload)),
		}
		recordBytes = make([]byte, RecordHeaderSize+len(payload))
		hdr.EncodeTo(recordBytes[:RecordHeaderSize])
		copy(recordBytes[RecordHeaderSize:], payload)
	}

	page, slotID, err := s.allocateRecordSlot(tx, recordBytes)
	if err != nil {
		return types.InvalidRecordID, err
	}

	loc := Location{
		Page: page.Header.ID,
		Slot: slotID,
	}
	if err := s.directory.Put(stableRecID, loc); err != nil {
		return types.InvalidRecordID, err
	}

	return stableRecID, nil
}

func (s *Store) ReadRecord(tx *txn.Transaction, recID types.RecordID) (*Header, []byte, error) {
	currID := recID
	visited := 0

	var snapID types.TxnID = types.MaxTxnID
	var currentTxnID types.TxnID = 0
	if tx != nil {
		snapID = tx.SnapshotID()
		currentTxnID = tx.ID()
	}

	for currID != types.InvalidRecordID && currID != 0 && visited < 1000 {
		visited++
		loc, found, err := s.directory.Get(currID)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			return nil, nil, types.ErrSlotNotFound
		}

		page, err := s.pager.Get(loc.Page)
		if err != nil {
			return nil, nil, err
		}

		sp := storage.WrapSlotted(page)
		recBytes, err := sp.Get(loc.Slot)
		if err != nil {
			return nil, nil, err
		}

		hdr, err := DecodeHeader(recBytes)
		if err != nil {
			return nil, nil, err
		}

		if hdr.IsVisible(snapID, s.manager.IsCommitted, currentTxnID) {
			if hdr.IsDeleted() {
				return nil, nil, types.ErrSlotNotFound
			}

			rawPayload := recBytes[RecordHeaderSize:]
			if (hdr.Flags & FlagOverflow) != 0 {
				if len(rawPayload) < 12 {
					return nil, nil, types.ErrInvalidFormat
				}
				firstPage := types.PageID(binary.BigEndian.Uint64(rawPayload[0:8]))
				totalLen := binary.BigEndian.Uint32(rawPayload[8:12])

				fullPayload, err := s.readOverflowChain(firstPage, totalLen)
				if err != nil {
					return nil, nil, err
				}
				return &hdr, fullPayload, nil
			}

			payloadCopy := make([]byte, len(rawPayload))
			copy(payloadCopy, rawPayload)
			return &hdr, payloadCopy, nil
		}

		// Follow version chain back
		currID = hdr.PrevVersion
	}

	return nil, nil, types.ErrSlotNotFound
}

// ReadRecordDirect retrieves record header and payload directly from page memory without copying.
func (s *Store) ReadRecordDirect(tx *txn.Transaction, recID types.RecordID) (Header, []byte, error) {
	currID := recID
	visited := 0

	var snapID types.TxnID = types.MaxTxnID
	var currentTxnID types.TxnID = 0
	if tx != nil {
		snapID = tx.SnapshotID()
		currentTxnID = tx.ID()
	}

	for currID != types.InvalidRecordID && currID != 0 && visited < 1000 {
		visited++
		loc, found, err := s.directory.Get(currID)
		if err != nil {
			return Header{}, nil, err
		}
		if !found {
			return Header{}, nil, types.ErrSlotNotFound
		}

		page, err := s.pager.Get(loc.Page)
		if err != nil {
			return Header{}, nil, err
		}

		sp := storage.WrapSlotted(page)
		recBytes, err := sp.GetDirect(loc.Slot)
		if err != nil {
			return Header{}, nil, err
		}

		hdr, err := DecodeHeader(recBytes)
		if err != nil {
			return Header{}, nil, err
		}

		if hdr.IsVisible(snapID, s.manager.IsCommitted, currentTxnID) {
			if hdr.IsDeleted() {
				return Header{}, nil, types.ErrSlotNotFound
			}

			rawPayload := recBytes[RecordHeaderSize:]
			if (hdr.Flags & FlagOverflow) != 0 {
				if len(rawPayload) < 12 {
					return Header{}, nil, types.ErrInvalidFormat
				}
				firstPage := types.PageID(binary.BigEndian.Uint64(rawPayload[0:8]))
				totalLen := binary.BigEndian.Uint32(rawPayload[8:12])

				fullPayload, err := s.readOverflowChain(firstPage, totalLen)
				if err != nil {
					return Header{}, nil, err
				}
				return hdr, fullPayload, nil
			}

			return hdr, rawPayload, nil
		}

		// Follow version chain back
		currID = hdr.PrevVersion
	}

	return Header{}, nil, types.ErrSlotNotFound
}

func (s *Store) DeleteRecord(tx *txn.Transaction, recID types.RecordID) error {
	_, err := s.WriteRecord(tx, nil, recID)
	return err
}

func (s *Store) allocateRecordSlot(tx *txn.Transaction, recordBytes []byte) (*storage.Page, types.SlotID, error) {
	needed := uint16(len(recordBytes) + storage.SlotSize)

	var page *storage.Page
	var sp *storage.SlottedPage
	var activeIdx = -1

	for i, p := range s.activePages {
		sp = storage.WrapSlotted(p)
		if sp.FreeSpace() >= needed {
			page = p
			activeIdx = i
			break
		}
	}

	if page == nil {
		var err error
		page, err = s.pager.Allocate(storage.PageRecord)
		if err != nil {
			return nil, types.InvalidSlotID, err
		}
		sp = storage.WrapSlotted(page)
		s.activePages = append(s.activePages, page)
		activeIdx = len(s.activePages) - 1
	}

	slotID, err := sp.Insert(recordBytes)
	if err != nil {
		return nil, types.InvalidSlotID, err
	}

	if err := tx.TrackDirty(page); err != nil {
		return nil, types.InvalidSlotID, err
	}

	// Remove page from active list if it cannot hold a minimal record
	if sp.FreeSpace() < RecordHeaderSize+storage.SlotSize {
		s.activePages = append(s.activePages[:activeIdx], s.activePages[activeIdx+1:]...)
	}

	return page, slotID, nil
}

func (s *Store) writeOverflowChain(tx *txn.Transaction, data []byte) (types.PageID, error) {
	var firstPageID types.PageID = types.InvalidPageID
	var prevOverflow *storage.OverflowPage

	remaining := data
	for len(remaining) > 0 {
		p, err := s.pager.Allocate(storage.PageOverflow)
		if err != nil {
			return types.InvalidPageID, err
		}
		op := storage.WrapOverflow(p)
		op.SetNextPage(types.InvalidPageID)
		n := op.WriteChunk(remaining)
		remaining = remaining[n:]

		if err := tx.TrackDirty(p); err != nil {
			return types.InvalidPageID, err
		}

		if firstPageID == types.InvalidPageID {
			firstPageID = p.Header.ID
		}
		if prevOverflow != nil {
			prevOverflow.SetNextPage(p.Header.ID)
		}
		prevOverflow = op
	}

	return firstPageID, nil
}

func (s *Store) readOverflowChain(firstPageID types.PageID, totalLength uint32) ([]byte, error) {
	result := make([]byte, 0, totalLength)
	currID := firstPageID

	for currID != types.InvalidPageID && len(result) < int(totalLength) {
		p, err := s.pager.Get(currID)
		if err != nil {
			return nil, err
		}
		op := storage.WrapOverflow(p)
		result = append(result, op.Payload()...)
		currID = op.NextPage()
	}

	if len(result) > int(totalLength) {
		result = result[:totalLength]
	}
	return result, nil
}
