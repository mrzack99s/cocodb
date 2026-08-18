package cocodb

import "cocodb/internal/types"

// Stats provides telemetry and operational metrics for CoCo DB.
type Stats struct {
	PageCount    int64
	CacheHits    uint64
	CacheMisses  uint64
	CacheHitRate float64
	LastLSN      types.LSN
	LastTxnID    types.TxnID
	ReadOnly     bool
}
