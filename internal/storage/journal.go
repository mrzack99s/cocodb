package storage

import (
	"bytes"
	"sync"

	"github.com/mrzack99s/cocodb/internal/types"
)

// Undo journal for write transactions.
//
// B+Tree, catalog, directory and free-list code changes cached pages in place, without
// going through the transaction. Rolling back therefore cannot just drop the record pages
// a transaction tracked: every other page it changed would keep the aborted state in the
// cache and reach the file at the next flush.
//
// While a journal is open, the pager keeps a before-image of each existing page the first
// time it is handed out (Get), and remembers each page it creates (Allocate). Rollback
// writes the before-images back into the same Page objects, drops the created pages and
// restores the structural meta fields. Pages are only captured while the single writer
// holds the write lock, so one journal at a time is enough.
//
// Writes made outside a transaction while a journal is open (non-transactional bucket
// helpers, queues) are captured too and are undone by a rollback.

type pageImage struct {
	page  *Page
	data  []byte
	dirty bool
}

type journal struct {
	mu      sync.Mutex
	before  map[types.PageID]pageImage
	created map[types.PageID]bool
	meta    MetaPage
}

// BeginJournal starts recording undo information for a write transaction.
func (p *DefaultPager) BeginJournal() {
	p.mu.RLock()
	meta := *p.meta
	p.mu.RUnlock()
	p.journal.Store(&journal{
		before:  make(map[types.PageID]pageImage),
		created: make(map[types.PageID]bool),
		meta:    meta,
	})
}

// ChangedPages returns every page the open write transaction created or changed, so
// the commit can write all of them to the WAL. Pages it only read are left out.
func (p *DefaultPager) ChangedPages() []*Page {
	j := p.journal.Load()
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*Page, 0, len(j.before)+len(j.created))
	for _, img := range j.before {
		if !bytes.Equal(img.data, img.page.Data) {
			out = append(out, img.page)
		}
	}
	for id := range j.created {
		if pg := p.cache.Get(id); pg != nil {
			p.cache.Unpin(id)
			out = append(out, pg)
		}
	}
	return out
}

// CommitJournal discards the undo information after a commit.
func (p *DefaultPager) CommitJournal() {
	p.journal.Store(nil)
}

// capture records the before-image of an existing page the first time it is handed out
// while a journal is open.
func (p *DefaultPager) capture(page *Page) {
	j := p.journal.Load()
	if j == nil || page == nil {
		return
	}
	id := page.Header.ID
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.created[id] {
		return
	}
	if _, ok := j.before[id]; ok {
		return
	}
	data := make([]byte, len(page.Data))
	copy(data, page.Data)
	j.before[id] = pageImage{page: page, data: data, dirty: page.Dirty}
}

// captureID records the before-image of page id, loading it if needed, before the pager
// replaces it with a new Page object.
func (p *DefaultPager) captureID(id types.PageID) {
	if p.journal.Load() == nil {
		return
	}
	if page, err := p.Get(id); err == nil {
		p.cache.Unpin(id)
		_ = page // Get captured it
	}
}

// markCreated records a page created by this transaction.
func (p *DefaultPager) markCreated(id types.PageID) {
	j := p.journal.Load()
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, existed := j.before[id]; !existed {
		j.created[id] = true
	}
}

// RollbackJournal undoes every page and meta change made since BeginJournal.
func (p *DefaultPager) RollbackJournal() {
	j := p.journal.Swap(nil)
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	for id := range j.created {
		p.cache.Remove(id)
	}
	for id, img := range j.before {
		pg := img.page
		copy(pg.Data, img.data)
		pg.ReadHeader()
		// Put the original object back if the cache now holds another one for this id.
		if cur := p.cache.Get(id); cur != pg {
			p.cache.Remove(id)
			p.cache.Put(pg)
		} else {
			p.cache.Unpin(id)
		}
		// Always rewrite the restored image: the aborted version may already have been
		// flushed to the file by a checkpoint.
		p.MarkDirty(pg)
	}

	// Restore the structural fields only. Generation, LSNs and the last transaction id
	// describe the file and the log, which have moved on; the record-id counter must never
	// go backwards.
	p.mu.Lock()
	p.meta.CatalogRoot = j.meta.CatalogRoot
	p.meta.RecordDirRoot = j.meta.RecordDirRoot
	p.meta.FreeListRoot = j.meta.FreeListRoot
	p.meta.NextPageID = j.meta.NextPageID
	p.mu.Unlock()
}
