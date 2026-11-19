package wal

import (
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

// RecoveryResult stores statistics from a crash recovery run.
type RecoveryResult struct {
	RecordsScanned int
	TxnsCommitted  int
	PagesRedone    int
	LastLSN        types.LSN
}

// Recover scans the WAL, identifies committed transactions, redoes page changes, and checkpoints.
func Recover(walManager *WAL, pager storage.Pager) (*RecoveryResult, error) {
	records, err := walManager.ReadAll()
	if err != nil {
		return nil, err
	}

	result := &RecoveryResult{
		RecordsScanned: len(records),
	}

	if len(records) == 0 {
		return result, nil
	}

	// Phase 1: Identify all committed transactions
	committedTxns := make(map[types.TxnID]bool)
	var maxLSN types.LSN
	var maxCommitted types.TxnID

	for _, rec := range records {
		if rec.Header.LSN > maxLSN {
			maxLSN = rec.Header.LSN
		}
		if rec.Header.Type == RecordTxnCommit {
			committedTxns[rec.Header.TxnID] = true
			if rec.Header.TxnID > maxCommitted {
				maxCommitted = rec.Header.TxnID
			}
		}
	}
	result.TxnsCommitted = len(committedTxns)
	result.LastLSN = maxLSN

	// Phase 2: Redo committed updates in LSN order
	for _, rec := range records {
		if rec.Header.Type == RecordMeta && rec.Meta != nil && committedTxns[rec.Header.TxnID] {
			// The roots of the catalog and record directory, and the allocation state,
			// as of this commit. Without them the redone pages would be unreachable.
			meta := pager.Meta()
			meta.CatalogRoot = rec.Meta.CatalogRoot
			meta.RecordDirRoot = rec.Meta.RecordDirRoot
			meta.FreeListRoot = rec.Meta.FreeListRoot
			if rec.Meta.NextPageID > meta.NextPageID {
				meta.NextPageID = rec.Meta.NextPageID
			}
			if rec.Meta.NextRecordID > meta.NextRecordID {
				meta.NextRecordID = rec.Meta.NextRecordID
			}
			continue
		}
		if rec.Header.Type == RecordPageUpdate {
			if committedTxns[rec.Header.TxnID] {
				if len(rec.PageData) == int(types.DefaultPageSize) {
					// Write raw page directly through pager (handles encryption transparently)
					if err := pager.WriteRawPage(rec.PageID, rec.PageData); err == nil {
						if rec.PageID >= pager.Meta().NextPageID {
							pager.Meta().NextPageID = rec.PageID + 1
						}
						// Refresh cache entry
						pager.Cache().Remove(rec.PageID)
						page, err := pager.Get(rec.PageID)
						if err == nil {
							pager.MarkDirty(page)
							result.PagesRedone++
						}
					}
				}
			}
		}
	}

	// Phase 3: Flush redone pages and checkpoint
	if err := pager.FlushAll(); err != nil {
		return nil, err
	}

	meta := pager.Meta()
	meta.LastLSN = maxLSN
	meta.LastCheckpointLSN = maxLSN
	// MVCC treats a record as committed when its transaction id is at most LastTxnID.
	// Without this the redone records stay invisible, as if never committed.
	if maxCommitted > meta.LastTxnID {
		meta.LastTxnID = maxCommitted
	}
	if err := pager.SaveMeta(); err != nil {
		return nil, err
	}

	// Truncate WAL after successful recovery
	if err := walManager.Truncate(); err != nil {
		return nil, err
	}

	return result, nil
}
