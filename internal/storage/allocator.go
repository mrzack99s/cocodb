package storage

import (
	"sync"

	"cocodb/internal/types"
)

// Allocator manages page allocation and reuse using the free list and NextPageID.
type Allocator struct {
	mu    sync.Mutex
	pager *DefaultPager
}

// NewAllocator creates a page Allocator.
func NewAllocator(p *DefaultPager) *Allocator {
	return &Allocator{
		pager: p,
	}
}

// Allocate allocates a new page of given type, either from FreeList or file extension.
func (a *Allocator) Allocate(pageType PageType) (*Page, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	meta := a.pager.Meta()

	// Try FreeList first
	if meta.FreeListRoot != types.InvalidPageID {
		flPage, err := a.pager.Get(meta.FreeListRoot)
		if err == nil {
			fl := WrapFreeList(flPage)
			if fl.Count() > 0 {
				reusedID, err := fl.Pop()
				if err == nil {
					a.pager.MarkDirty(flPage)
					// If free list page became empty and has a next page, update FreeListRoot
					if fl.Count() == 0 && fl.NextPage() != types.InvalidPageID {
						next := fl.NextPage()
						meta.FreeListRoot = next
					}
					// Initialize reused page
					reused := NewPage(reusedID, pageType)
					a.pager.Cache().Put(reused)
					a.pager.MarkDirty(reused)
					return reused, nil
				}
			}
		}
	}

	// Otherwise extend database
	newID := meta.NextPageID
	meta.NextPageID++

	page := NewPage(newID, pageType)
	a.pager.Cache().Put(page)
	a.pager.MarkDirty(page)
	return page, nil
}

// Free releases a page back to the FreeList.
func (a *Allocator) Free(id types.PageID) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	meta := a.pager.Meta()

	if meta.FreeListRoot == types.InvalidPageID {
		// Allocate a new FreeListPage using the freed page itself
		flPage := NewPage(id, PageFreeList)
		fl := WrapFreeList(flPage)
		fl.SetNextPage(types.InvalidPageID)
		fl.SetCount(0)
		meta.FreeListRoot = id
		a.pager.Cache().Put(flPage)
		a.pager.MarkDirty(flPage)
		return nil
	}

	// Try pushing to current FreeListRoot
	flPage, err := a.pager.Get(meta.FreeListRoot)
	if err != nil {
		return err
	}
	fl := WrapFreeList(flPage)
	if fl.Count() < uint32(MaxFreeListEntries) {
		_ = fl.Push(id)
		a.pager.MarkDirty(flPage)
		return nil
	}

	// Root is full, chain the new freed page as new FreeListRoot
	newFlPage := NewPage(id, PageFreeList)
	newFl := WrapFreeList(newFlPage)
	newFl.SetNextPage(meta.FreeListRoot)
	newFl.SetCount(0)
	meta.FreeListRoot = id
	a.pager.Cache().Put(newFlPage)
	a.pager.MarkDirty(newFlPage)
	return nil
}
