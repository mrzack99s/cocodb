package storage

import (
	"fmt"
	"sync"

	"cocodb/internal/file"
	"cocodb/internal/types"
)

// Pager defines the core page storage interface.
var pagePool = sync.Pool{
	New: func() any {
		b := make([]byte, 16384)
		return &b
	},
}

type Pager interface {
	Get(id types.PageID) (*Page, error)
	Allocate(pageType PageType) (*Page, error)
	Free(id types.PageID) error
	MarkDirty(p *Page)
	Flush(id types.PageID) error
	FlushAll() error
	Close() error
	Meta() *MetaPage
	SaveMeta() error
	Cache() *PageCache
	Backend() file.Backend
	SetWALFlusher(fn func(lsn types.LSN) error)
}

// DefaultPager implements the Pager interface.
type DefaultPager struct {
	mu             sync.RWMutex
	backend        file.Backend
	cache          *PageCache
	allocator      *Allocator
	meta           *MetaPage
	activeMetaSlot int // 0 for Meta A (Page 0), 1 for Meta B (Page 1)
	pageSize       uint32
	readOnly       bool
	walFlusher     func(lsn types.LSN) error
}

// OpenPager opens an existing or initializes a new database pager.
func OpenPager(backend file.Backend, maxCacheBytes int64, readOnly bool) (*DefaultPager, error) {
	size, err := backend.Size()
	if err != nil {
		return nil, err
	}

	p := &DefaultPager{
		backend:  backend,
		pageSize: types.DefaultPageSize,
		readOnly: readOnly,
	}
	p.cache = NewPageCache(maxCacheBytes, p.pageSize)
	p.allocator = NewAllocator(p)

	if size == 0 {
		if readOnly {
			return nil, types.ErrInvalidFormat
		}
		// Initialize brand new database
		metaA := NewInitialMetaPage(p.pageSize)
		metaA.Generation = 1

		metaB := NewInitialMetaPage(p.pageSize)
		metaB.Generation = 0

		bufA := metaA.Encode(p.pageSize)
		bufB := metaB.Encode(p.pageSize)

		if _, err := backend.WriteAt(bufA, 0); err != nil {
			return nil, err
		}
		if _, err := backend.WriteAt(bufB, int64(p.pageSize)); err != nil {
			return nil, err
		}
		if err := backend.Sync(); err != nil {
			return nil, err
		}

		p.meta = metaA
		p.activeMetaSlot = 0
		return p, nil
	}

	// Read both Meta A and Meta B
	bufA := make([]byte, p.pageSize)
	bufB := make([]byte, p.pageSize)

	_, errA := backend.ReadAt(bufA, 0)
	_, errB := backend.ReadAt(bufB, int64(p.pageSize))

	var metaA, metaB *MetaPage
	if errA == nil {
		metaA, _ = DecodeMetaPage(bufA)
	}
	if errB == nil {
		metaB, _ = DecodeMetaPage(bufB)
	}

	if metaA == nil && metaB == nil {
		return nil, types.ErrCorruptMeta
	}

	if metaA != nil && metaB != nil {
		if metaA.Generation >= metaB.Generation {
			p.meta = metaA
			p.activeMetaSlot = 0
		} else {
			p.meta = metaB
			p.activeMetaSlot = 1
		}
	} else if metaA != nil {
		p.meta = metaA
		p.activeMetaSlot = 0
	} else {
		p.meta = metaB
		p.activeMetaSlot = 1
	}

	p.pageSize = p.meta.PageSize
	return p, nil
}

func (p *DefaultPager) Meta() *MetaPage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.meta
}

func (p *DefaultPager) Cache() *PageCache {
	return p.cache
}

func (p *DefaultPager) Backend() file.Backend {
	return p.backend
}

func (p *DefaultPager) SetWALFlusher(fn func(lsn types.LSN) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.walFlusher = fn
}

// Get retrieves a page by ID (checking cache first, then disk).
func (p *DefaultPager) Get(id types.PageID) (*Page, error) {
	if cached := p.cache.Get(id); cached != nil {
		return cached, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double check cache under lock
	if cached := p.cache.Get(id); cached != nil {
		return cached, nil
	}

	offset := int64(id) * int64(p.pageSize)

	bptr := pagePool.Get().(*[]byte)
	buf := *bptr
	if len(buf) != int(p.pageSize) {
		buf = make([]byte, p.pageSize)
	}

	if _, err := p.backend.ReadAt(buf, offset); err != nil {
		pagePool.Put(&buf)
		return nil, fmt.Errorf("%w: reading page %d: %v", types.ErrPageNotFound, id, err)
	}

	page := &Page{
		Data:  buf,
		Dirty: false,
		Pins:  1,
	}
	page.ReadHeader()

	if !page.ValidateChecksum() {
		return nil, fmt.Errorf("%w: checksum mismatch on page %d", types.ErrChecksumFailed, id)
	}

	p.cache.Put(page)
	return page, nil
}

func (p *DefaultPager) Allocate(pageType PageType) (*Page, error) {
	return p.allocator.Allocate(pageType)
}

func (p *DefaultPager) Free(id types.PageID) error {
	return p.allocator.Free(id)
}

func (p *DefaultPager) MarkDirty(page *Page) {
	page.Dirty = true
	p.cache.MarkDirty(page.Header.ID, page.Header.LSN)
}

// Flush writes a single dirty page to disk with CRC32 seal, after enforcing WAL durability.
func (p *DefaultPager) Flush(id types.PageID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	page := p.cache.Get(id)
	if page == nil {
		return nil
	}
	defer p.cache.Unpin(id)

	if !page.Dirty {
		return nil
	}

	// WAL Safety Rule: Enforce that WAL is durable up to page.Header.LSN before flushing page
	if p.walFlusher != nil && page.Header.LSN > 0 {
		if err := p.walFlusher(page.Header.LSN); err != nil {
			return fmt.Errorf("wal flush failed before page %d write: %w", id, err)
		}
	}

	page.Seal()
	offset := int64(id) * int64(p.pageSize)
	if _, err := p.backend.WriteAt(page.Data, offset); err != nil {
		return err
	}
	page.Dirty = false
	return nil
}

// FlushAll flushes all dirty pages from cache, then syncs backend, then updates meta.
func (p *DefaultPager) FlushAll() error {
	dirtyEntries := p.cache.GetAllDirty()
	for _, entry := range dirtyEntries {
		if err := p.Flush(entry.ID); err != nil {
			return err
		}
	}
	if err := p.backend.Sync(); err != nil {
		return err
	}
	return p.SaveMeta()
}

// SaveMeta persists the active meta page to the alternate meta page slot.
func (p *DefaultPager) SaveMeta() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.readOnly {
		return nil
	}

	p.meta.Generation++
	nextSlot := 1 - p.activeMetaSlot
	buf := p.meta.Encode(p.pageSize)
	offset := int64(nextSlot) * int64(p.pageSize)

	if _, err := p.backend.WriteAt(buf, offset); err != nil {
		return err
	}
	if err := p.backend.Sync(); err != nil {
		return err
	}

	p.activeMetaSlot = nextSlot
	return nil
}

func (p *DefaultPager) Close() error {
	if !p.readOnly {
		_ = p.FlushAll()
	}
	return p.backend.Close()
}
