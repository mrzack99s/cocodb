package cocodb

import (
	"errors"

	internalQueue "github.com/mrzack99s/cocodb/internal/queue"
)

var (
	ErrNotFound      = errors.New("coco: not found")
	ErrExists        = errors.New("coco: already exists")
	ErrReadOnly      = errors.New("coco: database is read-only")
	ErrTxnClosed     = errors.New("coco: transaction closed")
	ErrConflict      = errors.New("coco: transaction conflict")
	ErrCorrupt       = errors.New("coco: database corruption")
	ErrInvalidFormat = errors.New("coco: invalid database format")
	ErrUnsupported   = errors.New("coco: unsupported operation")
	ErrDatabaseFull  = errors.New("coco: database memory/query limit exceeded")
	ErrInvalidSchema = errors.New("coco: document does not match collection schema")
	ErrKeyTooLarge   = errors.New("coco: key exceeds maximum allowed size")
	ErrTxnReadOnly   = errors.New("coco: cannot write in read-only transaction")

	// Queue & PubSub errors
	ErrDuplicateMessage   = internalQueue.ErrDuplicateMessage
	ErrQueueEmpty         = internalQueue.ErrQueueEmpty
	ErrMessageNotFound    = internalQueue.ErrMessageNotFound
	ErrMessageExpired     = internalQueue.ErrMessageExpired
	ErrMaxRetriesExceeded = internalQueue.ErrMaxRetriesExceeded
)
