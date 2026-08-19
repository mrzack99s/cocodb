package cocodb

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/mrzack99s/cocodb/document"
	"github.com/mrzack99s/cocodb/internal/backup"
	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/catalog"
	"github.com/mrzack99s/cocodb/internal/cson"
	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/index"
	"github.com/mrzack99s/cocodb/internal/integrity"
	"github.com/mrzack99s/cocodb/internal/maintenance"
	internalQueue "github.com/mrzack99s/cocodb/internal/queue"
	"github.com/mrzack99s/cocodb/internal/record"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
	"github.com/mrzack99s/cocodb/internal/wal"
	"github.com/mrzack99s/cocodb/kv"
	"github.com/mrzack99s/cocodb/pubsub"
	"github.com/mrzack99s/cocodb/queue"
)

// DB represents a CoCo multi-model embedded database instance.
type DB struct {
	mu              sync.RWMutex
	path            string
	backend         file.Backend
	lock            *file.FileLock
	pager           storage.Pager
	wal             *wal.WAL
	tm              *txn.TxnManager
	catalog         *catalog.Catalog
	dir             *record.Directory
	store           *record.Store
	ttlIndex        *index.TTLIndex
	scheduler       *maintenance.Scheduler
	trees           map[types.PageID]*btree.BTree
	colDicts        map[string]*cson.FieldDictionary
	queues          map[string]*queue.Queue
	pubsub          *pubsub.PubSub
	modelDBs        map[Model]*DB
	clusterStatusFn func() any
	opts            Options
	closed          bool
}

// Open opens an existing or creates a new CoCo database.
func Open(path string, opts ...Option) (*DB, error) {
	cfg := DefaultOptions()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.MultiWriter {
		cfg.Background = false
	}

	var backend file.Backend
	var lock *file.FileLock

	usesDisk := cfg.BackendFactory == nil && (cfg.Storage == StorageDisk || (cfg.Storage == StorageAuto && path != ":memory:" && path != ""))
	if usesDisk {
		var err error
		if !cfg.ReadOnly {
			lock, err = file.AcquireLock(path)
			if err != nil {
				return nil, err
			}
		}
	}
	backend, err := openBackend(cfg, path, false)
	if err != nil {
		if lock != nil {
			_ = lock.Release()
		}
		return nil, err
	}

	pager, err := storage.OpenPager(backend, cfg.MemoryLimit, cfg.ReadOnly)
	if err != nil {
		_ = backend.Close()
		if lock != nil {
			_ = lock.Release()
		}
		return nil, err
	}

	// Open or create WAL
	walPath := path
	if usesDisk || cfg.BackendFactory != nil {
		walPath += "-wal"
	}
	walBackend, err := openBackend(cfg, walPath, true)
	if err != nil {
		_ = pager.Close()
		if lock != nil {
			_ = lock.Release()
		}
		return nil, err
	}

	walManager, err := wal.OpenWAL(walBackend, pager.Meta().LastLSN)
	if err != nil {
		_ = walBackend.Close()
		_ = pager.Close()
		if lock != nil {
			_ = lock.Release()
		}
		return nil, err
	}

	// Run crash recovery if WAL has records
	if !cfg.ReadOnly {
		_, err = wal.Recover(walManager, pager)
		if err != nil {
			_ = walManager.Close()
			_ = pager.Close()
			if lock != nil {
				_ = lock.Release()
			}
			return nil, fmt.Errorf("crash recovery failed: %w", err)
		}
	}

	tmSyncMode := txn.SyncNormal
	switch cfg.SyncMode {
	case SyncFull:
		tmSyncMode = txn.SyncFull
	case SyncOff:
		tmSyncMode = txn.SyncOff
	}
	tm := txn.NewTxnManager(pager, walManager, tmSyncMode)

	cat, err := catalog.NewCatalog(pager, pager.Meta().CatalogRoot)
	if err != nil {
		_ = walManager.Close()
		_ = pager.Close()
		if lock != nil {
			_ = lock.Release()
		}
		return nil, err
	}
	pager.Meta().CatalogRoot = cat.Root()

	dir := record.NewDirectory(pager, pager.Meta().RecordDirRoot)
	pager.Meta().RecordDirRoot = dir.Root()

	store := record.NewStore(pager, dir, tm)
	ttlIndex := index.NewTTLIndex(pager, types.InvalidPageID)

	db := &DB{
		path:     path,
		backend:  backend,
		lock:     lock,
		pager:    pager,
		wal:      walManager,
		tm:       tm,
		catalog:  cat,
		dir:      dir,
		store:    store,
		ttlIndex: ttlIndex,
		trees:    make(map[types.PageID]*btree.BTree),
		colDicts: make(map[string]*cson.FieldDictionary),
		queues:   make(map[string]*queue.Queue),
		pubsub:   pubsub.New(256),
		modelDBs: make(map[Model]*DB),
		opts:     cfg,
	}

	// Start background scheduler if enabled
	if cfg.Background && !cfg.ReadOnly {
		db.scheduler = maintenance.NewScheduler(
			pager,
			walManager,
			ttlIndex,
			func(objID types.ObjectID, keyOrID []byte) error {
				return nil
			},
			cfg.CleanInterval,
		)
		db.scheduler.Start()
	}

	// A model override owns an independent database engine. This intentionally
	// keeps volatile KV data out of the durable document file (and vice versa).
	for model, storageCfg := range cfg.ModelStorage {
		if storageCfg.Kind == cfg.Storage && storageCfg.Factory == nil {
			continue
		}
		modelPath := path + "-" + string(model)
		childOpts := []Option{
			MemoryLimit(cfg.MemoryLimit),
			SyncMode(cfg.SyncMode),
			Background(cfg.Background),
			Storage(storageCfg.Kind),
		}
		if cfg.ReadOnly {
			childOpts = append(childOpts, ReadOnly())
		}
		if storageCfg.Factory != nil {
			childOpts = append(childOpts, CustomStorage(storageCfg.Factory))
		}
		if cfg.MultiWriter {
			childOpts = append(childOpts, MultiWriter(), WriterTimeout(cfg.WriterTimeout))
		}
		child, err := Open(modelPath, childOpts...)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("open %s storage: %w", model, err)
		}
		db.modelDBs[model] = child
	}
	if cfg.MultiWriter && db.lock != nil {
		// The lock is reacquired for each isolated write transaction instead
		// of being held for the lifetime of this process.
		_ = db.lock.Release()
		db.lock = nil
	}

	return db, nil
}

func (db *DB) isolatedWriter(fn func(tx *Tx) error) error {
	deadline := time.Now().Add(db.opts.WriterTimeout)
	for {
		cfg := db.opts
		cfg.MultiWriter = false
		cfg.Background = false
		cfg.ModelStorage = maps.Clone(db.opts.ModelStorage)
		writer, err := Open(db.path, func(o *Options) { *o = cfg })
		if err == nil {
			err = writer.Update(fn)
			closeErr := writer.Close()
			if err != nil {
				return err
			}
			return closeErr
		}
		if !errors.Is(err, file.ErrDatabaseLocked) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (db *DB) modelDB(model Model) *DB {
	if child := db.modelDBs[model]; child != nil {
		return child
	}
	return db
}

func (db *DB) getOrOpenTree(root types.PageID) *btree.BTree {
	db.mu.RLock()
	if t, ok := db.trees[root]; ok {
		db.mu.RUnlock()
		return t
	}
	db.mu.RUnlock()

	db.mu.Lock()
	defer db.mu.Unlock()

	if t, ok := db.trees[root]; ok {
		return t
	}
	t := btree.NewBTree(db.pager, root)
	db.trees[root] = t
	return t
}

func (db *DB) getOrOpenDict(name string) *cson.FieldDictionary {
	db.mu.RLock()
	if d, ok := db.colDicts[name]; ok {
		db.mu.RUnlock()
		return d
	}
	db.mu.RUnlock()

	db.mu.Lock()
	defer db.mu.Unlock()

	if d, ok := db.colDicts[name]; ok {
		return d
	}
	d := cson.NewFieldDictionary()
	if obj, ok := db.catalog.GetObject(catalog.ObjectCollection, name); ok && len(obj.ExtraData) > 0 {
		_ = d.Decode(obj.ExtraData)
	}
	db.colDicts[name] = d
	return d
}

// View executes a read-only transaction function.
func (db *DB) View(fn func(tx *Tx) error) error {
	tx, err := db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return fn(tx)
}

// Update executes a read-write transaction function.
func (db *DB) Update(fn func(tx *Tx) error) error {
	if db.opts.ReadOnly {
		return ErrReadOnly
	}
	if db.opts.MultiWriter {
		return db.isolatedWriter(fn)
	}
	tx, err := db.Begin(false)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

var txPool = sync.Pool{
	New: func() interface{} {
		return &Tx{}
	},
}

// Begin starts a new transaction.
func (db *DB) Begin(readOnly bool) (*Tx, error) {
	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ErrTxnClosed
	}
	db.mu.RUnlock()

	internalTx, err := db.tm.Begin(readOnly)
	if err != nil {
		return nil, err
	}

	tx := txPool.Get().(*Tx)
	tx.db = db
	tx.internalTx = internalTx
	tx.buckets = nil
	tx.collections = nil
	tx.childTxs = nil

	return tx, nil
}

// Bucket returns a Bucket helper for quick point operations.
func (db *DB) Bucket(name string) *kv.Bucket {
	if target := db.modelDB(ModelKV); target != db {
		return target.Bucket(name)
	}
	obj, ok := db.catalog.GetObject(catalog.ObjectBucket, name)
	var root types.PageID = types.InvalidPageID
	var objID types.ObjectID = types.InvalidObjectID
	if ok {
		root = obj.Root
		objID = obj.ID
	} else if !db.opts.ReadOnly {
		obj = &catalog.Object{
			Type: catalog.ObjectBucket,
			Name: name,
			Root: types.InvalidPageID,
		}
		_ = db.catalog.PutObject(obj)
		root = obj.Root
		objID = obj.ID
	}
	tree := db.getOrOpenTree(root)
	return kv.NewBucket(name, objID, tree, nil, db.ttlIndex)
}

// Collection returns a Collection helper for document operations.
func (db *DB) Collection(name string) *document.Collection {
	if target := db.modelDB(ModelDocument); target != db {
		return target.Collection(name)
	}
	obj, ok := db.catalog.GetObject(catalog.ObjectCollection, name)
	var root types.PageID = types.InvalidPageID
	var objID types.ObjectID = types.InvalidObjectID
	dict := db.getOrOpenDict(name)

	if ok {
		root = obj.Root
		objID = obj.ID
	} else if !db.opts.ReadOnly {
		obj = &catalog.Object{
			Type:      catalog.ObjectCollection,
			Name:      name,
			Root:      types.InvalidPageID,
			ExtraData: dict.Encode(),
		}
		_ = db.catalog.PutObject(obj)
		root = obj.Root
		objID = obj.ID
	}
	return document.NewCollection(name, objID, db.pager, nil, db.store, dict, root)
}

// ListBuckets returns the names of all Key/Value buckets in the database.
func (db *DB) ListBuckets() []string {
	if target := db.modelDB(ModelKV); target != db {
		return target.ListBuckets()
	}
	objs := db.catalog.ListObjects(catalog.ObjectBucket)
	res := make([]string, len(objs))
	for i, o := range objs {
		res[i] = o.Name
	}
	return res
}

// ListCollections returns the names of all Document collections in the database.
func (db *DB) ListCollections() []string {
	if target := db.modelDB(ModelDocument); target != db {
		return target.ListCollections()
	}
	objs := db.catalog.ListObjects(catalog.ObjectCollection)
	res := make([]string, len(objs))
	for i, o := range objs {
		res[i] = o.Name
	}
	return res
}

// Queue returns or opens a transactional task queue handle.
func (db *DB) Queue(name string, opts ...queue.Option) *queue.Queue {
	if target := db.modelDB(ModelQueue); target != db {
		return target.Queue(name, opts...)
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	if q, ok := db.queues[name]; ok {
		return q
	}

	obj, ok := db.catalog.GetObject(catalog.ObjectQueue, name)
	var root types.PageID = types.InvalidPageID
	if ok {
		root = obj.Root
	} else if !db.opts.ReadOnly {
		obj = &catalog.Object{
			Type: catalog.ObjectQueue,
			Name: name,
			Root: types.InvalidPageID,
		}
		_ = db.catalog.PutObject(obj)
		root = obj.Root
	}

	var tree *btree.BTree
	if t, exists := db.trees[root]; exists {
		tree = t
	} else {
		tree = btree.NewBTree(db.pager, root)
		db.trees[root] = tree
	}

	qCfg := internalQueue.DefaultConfig()
	qEngine := internalQueue.NewQueueEngine(name, qCfg, tree, db.pager, false)
	q := queue.New(qEngine)
	db.queues[name] = q
	return q
}

// PubSub returns the shared high-performance Pub/Sub broker.
func (db *DB) PubSub() *pubsub.PubSub {
	return db.pubsub
}

// Topic returns a scoped helper for publishing and subscribing to a topic.
func (db *DB) Topic(name string) *pubsub.Topic {
	return db.pubsub.Topic(name)
}

// Publish broadcasts a payload to all topic subscribers.
func (db *DB) Publish(ctx context.Context, topic string, payload []byte, opts ...pubsub.Option) (int, error) {
	return db.pubsub.Publish(ctx, topic, payload, opts...)
}

// Subscribe subscribes to a topic or wildcard pattern.
func (db *DB) Subscribe(ctx context.Context, topicPattern string, opts ...pubsub.Option) *pubsub.Subscription {
	return db.pubsub.Subscribe(ctx, topicPattern, opts...)
}

// ListQueues returns the names of all persistent Queues registered in the catalog.
func (db *DB) ListQueues() []string {
	if target := db.modelDB(ModelQueue); target != db {
		return target.ListQueues()
	}
	objs := db.catalog.ListObjects(catalog.ObjectQueue)
	res := make([]string, len(objs))
	for i, o := range objs {
		res[i] = o.Name
	}
	return res
}

// Check validates database and storage kernel integrity.
func (db *DB) Check(ctx context.Context) (*integrity.Report, error) {
	return integrity.Check(ctx, db.pager, db.catalog, db.dir)
}

// Backup creates a point-in-time snapshot backup to dstPath.
func (db *DB) Backup(ctx context.Context, dstPath string) error {
	return backup.Backup(ctx, db.pager, dstPath)
}

// Stats returns runtime statistics.
func (db *DB) Stats() Stats {
	hits, misses, hitRate := db.pager.Cache().Stats()
	meta := db.pager.Meta()
	return Stats{
		PageCount:    int64(meta.NextPageID),
		CacheHits:    hits,
		CacheMisses:  misses,
		CacheHitRate: hitRate,
		LastLSN:      meta.LastLSN,
		LastTxnID:    meta.LastTxnID,
		ReadOnly:     db.opts.ReadOnly,
	}
}

// Close closes the database cleanly.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}
	db.closed = true

	if db.scheduler != nil {
		db.scheduler.Stop()
	}
	for _, child := range db.modelDBs {
		if err := child.Close(); err != nil {
			return err
		}
	}

	// Close and stop background workers of all active queues
	for _, q := range db.queues {
		_ = q.Close()
	}

	// Persist all dictionary changes to catalog
	for name, dict := range db.colDicts {
		if obj, ok := db.catalog.GetObject(catalog.ObjectCollection, name); ok {
			obj.ExtraData = dict.Encode()
			_ = db.catalog.PutObject(obj)
		}
	}

	if db.opts.MultiWriter {
		for _, child := range db.modelDBs {
			_ = child.Close()
		}
		// This handle is read-mostly and may have stale metadata. Do not let
		// Pager.Close checkpoint it over a newer multi-process commit.
		_ = db.wal.Close()
		return db.backend.Close()
	}

	var errs []error
	if err := db.pager.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := db.wal.Close(); err != nil {
		errs = append(errs, err)
	}
	if db.lock != nil {
		_ = db.lock.Release()
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// SetClusterStatusProvider registers a live cluster status function.
func (db *DB) SetClusterStatusProvider(fn func() any) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.clusterStatusFn = fn
}

// ClusterStatus returns the live cluster status if active, or nil.
func (db *DB) ClusterStatus() any {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.clusterStatusFn == nil {
		return nil
	}
	return db.clusterStatusFn()
}
