package integrity

import (
	"context"
	"fmt"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/catalog"
	"github.com/mrzack99s/cocodb/internal/record"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

// Report contains results of a database integrity verification.
type Report struct {
	Valid        bool
	PagesChecked int
	Errors       []string
	Warnings     []string
}

// Check performs full integrity checking across meta, pager, btrees, and records.
func Check(ctx context.Context, pager storage.Pager, cat *catalog.Catalog, dir *record.Directory) (*Report, error) {
	rep := &Report{
		Valid: true,
	}

	// Flush and seal all in-memory dirty pages before validating checksums
	if err := pager.FlushAll(); err != nil {
		rep.Valid = false
		rep.Errors = append(rep.Errors, fmt.Sprintf("flush all failed: %v", err))
	}

	meta := pager.Meta()
	if string(meta.Magic[:]) != types.MagicHeader {
		rep.Valid = false
		rep.Errors = append(rep.Errors, "invalid meta magic")
	}

	// Verify all allocated pages up to NextPageID
	for pID := types.PageID(2); pID < meta.NextPageID; pID++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		page, err := pager.Get(pID)
		if err != nil {
			rep.Valid = false
			rep.Errors = append(rep.Errors, fmt.Sprintf("page %d unreadable: %v", pID, err))
			continue
		}

		if !page.ValidateChecksum() {
			rep.Valid = false
			rep.Errors = append(rep.Errors, fmt.Sprintf("page %d checksum mismatch", pID))
		}
		rep.PagesChecked++
	}

	// Verify Catalog B+Tree
	if cat != nil && cat.Root() != types.InvalidPageID {
		catTree := btree.NewBTree(pager, cat.Root())
		if err := catTree.VerifyTree(); err != nil {
			rep.Valid = false
			rep.Errors = append(rep.Errors, fmt.Sprintf("catalog btree corrupt: %v", err))
		}
	}

	// Verify Record Directory B+Tree
	if dir != nil && dir.Root() != types.InvalidPageID {
		dirTree := btree.NewBTree(pager, dir.Root())
		if err := dirTree.VerifyTree(); err != nil {
			rep.Valid = false
			rep.Errors = append(rep.Errors, fmt.Sprintf("record directory btree corrupt: %v", err))
		}
	}

	return rep, nil
}
