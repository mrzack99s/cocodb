package catalog

import (
	"fmt"
	"sync"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

// Catalog manages system metadata (Buckets, Collections, Indexes).
type Catalog struct {
	mu      sync.RWMutex
	tree    *btree.BTree
	pager   storage.Pager
	objects map[string]*Object
	nextOID types.ObjectID
}

// NewCatalog opens or creates the system catalog.
func NewCatalog(pager storage.Pager, root types.PageID) (*Catalog, error) {
	c := &Catalog{
		tree:    btree.NewBTree(pager, root),
		pager:   pager,
		objects: make(map[string]*Object),
		nextOID: 1,
	}

	// Scan all objects from BTree if root is valid
	if root != types.InvalidPageID {
		cur := btree.NewCursor(c.tree)
		defer cur.Close()

		if cur.First() {
			for cur.Valid() {
				obj, err := DecodeObject(cur.Value())
				if err == nil {
					key := fmt.Sprintf("%d:%s", obj.Type, obj.Name)
					c.objects[key] = obj
					if obj.ID >= c.nextOID {
						c.nextOID = obj.ID + 1
					}
				}
				if !cur.Next() {
					break
				}
			}
		}
	}

	return c, nil
}

// Root returns the catalog root PageID.
func (c *Catalog) Root() types.PageID {
	return c.tree.Root()
}

func (c *Catalog) makeKey(objType ObjectType, name string) []byte {
	return []byte(fmt.Sprintf("%02d:%s", objType, name))
}

// GetObject looks up an object by type and name.
func (c *Catalog) GetObject(objType ObjectType, name string) (*Object, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := fmt.Sprintf("%d:%s", objType, name)
	obj, ok := c.objects[key]
	return obj, ok
}

// PutObject stores or updates an object in the catalog.
func (c *Catalog) PutObject(obj *Object) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if obj.ID == types.InvalidObjectID {
		obj.ID = c.nextOID
		c.nextOID++
	}

	k := c.makeKey(obj.Type, obj.Name)
	v := obj.Encode()

	if err := c.tree.Insert(k, v); err != nil {
		return err
	}

	c.pager.Meta().CatalogRoot = c.tree.Root()
	key := fmt.Sprintf("%d:%s", obj.Type, obj.Name)
	c.objects[key] = obj
	return nil
}

// DropObject deletes an object from the catalog.
func (c *Catalog) DropObject(objType ObjectType, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := c.makeKey(objType, name)
	if err := c.tree.Delete(k); err != nil {
		return err
	}

	c.pager.Meta().CatalogRoot = c.tree.Root()
	key := fmt.Sprintf("%d:%s", objType, name)
	delete(c.objects, key)
	return nil
}

// ListObjects returns all objects of a given type.
func (c *Catalog) ListObjects(objType ObjectType) []*Object {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*Object
	for _, obj := range c.objects {
		if obj.Type == objType {
			result = append(result, obj)
		}
	}
	return result
}
