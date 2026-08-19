package cocodb

import (
	"fmt"
	"time"

	"github.com/mrzack99s/cocodb/internal/file"
)

type ProfileType int

const (
	Tiny ProfileType = iota
	Balanced
	Performance
)

type SyncModeType int

const (
	SyncFull SyncModeType = iota
	SyncNormal
	SyncOff
)

// StorageKind selects the physical storage used by the database. The selected
// backend is shared by every model, including Key/Value buckets.
type StorageKind int

const (
	// StorageAuto uses memory for :memory: databases and disk for file paths.
	StorageAuto StorageKind = iota
	// StorageMemory keeps all database data in RAM.
	StorageMemory
	// StorageDisk persists database data at the path passed to Open.
	StorageDisk
)

// Model identifies a persistent CoCoDB model that can use its own storage.
type Model string

const (
	ModelKV       Model = "kv"
	ModelDocument Model = "document"
	ModelQueue    Model = "queue"
)

// ModelStorageConfig overrides the default backend for one model. A custom
// factory takes precedence over Kind when it is set.
type ModelStorageConfig struct {
	Kind    StorageKind
	Factory BackendFactory
}

// Backend is the public contract for a custom storage implementation. It is
// structurally compatible with the internal pager backend, so applications can
// plug in encrypted, remote, or test storage without importing an internal
// package.
type Backend interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
	Sync() error
	Truncate(int64) error
	Size() (int64, error)
	Close() error
}

// BackendFactory creates a backend for a database and its WAL. wal is true for
// the write-ahead log. The path supplied for WAL storage ends in "-wal".
type BackendFactory func(path string, readOnly, wal bool) (Backend, error)

// ProfileConfig contains the tunables supplied by CustomProfile. It starts
// from the normal defaults and only overrides positive duration/size values;
// SyncMode and Background are always applied so callers can explicitly choose
// SyncFull and disable maintenance.
type ProfileConfig struct {
	MemoryLimit   int64
	SyncMode      SyncModeType
	Background    bool
	CleanInterval time.Duration
}

type Options struct {
	Profile        ProfileType
	SyncMode       SyncModeType
	MemoryLimit    int64
	ReadOnly       bool
	EncryptionKey  []byte
	KeyID          string
	Background     bool
	CleanInterval  time.Duration
	EnableStudio   bool
	StudioAddr     string
	MultiWriter    bool
	WriterTimeout  time.Duration
	Storage        StorageKind
	BackendFactory BackendFactory
	ModelStorage   map[Model]ModelStorageConfig
}

type Option func(*Options)

func DefaultOptions() Options {
	return Options{
		Profile:       Balanced,
		SyncMode:      SyncNormal,
		MemoryLimit:   64 * 1024 * 1024, // 64 MB
		ReadOnly:      false,
		Background:    true,
		CleanInterval: 5 * time.Second,
		EnableStudio:  false,
		StudioAddr:    ":8787",
		WriterTimeout: 30 * time.Second,
		Storage:       StorageAuto,
		ModelStorage:  make(map[Model]ModelStorageConfig),
	}
}

// MultiWriter enables safe writes from multiple processes opening the same
// database path. Each Update uses a short-lived, exclusively locked engine so
// it always recovers and writes the latest on-disk state.
func MultiWriter() Option {
	return func(o *Options) {
		o.MultiWriter = true
		// Background workers retain cached pages, which is incompatible with
		// independently committing processes.
		o.Background = false
	}
}

// WriterTimeout limits how long MultiWriter waits for another process to
// finish its write transaction.
func WriterTimeout(d time.Duration) Option {
	return func(o *Options) { o.WriterTimeout = d }
}

// CustomProfile applies application-defined performance and durability defaults.
// Individual options passed after it take precedence.
func CustomProfile(p ProfileConfig) Option {
	return func(o *Options) {
		if p.MemoryLimit > 0 {
			o.MemoryLimit = p.MemoryLimit
		}
		o.SyncMode = p.SyncMode
		o.Background = p.Background
		if p.CleanInterval > 0 {
			o.CleanInterval = p.CleanInterval
		}
	}
}

func Profile(p ProfileType) Option {
	return func(o *Options) {
		o.Profile = p
		switch p {
		case Tiny:
			o.MemoryLimit = 8 * 1024 * 1024
		case Balanced:
			o.MemoryLimit = 64 * 1024 * 1024
		case Performance:
			o.MemoryLimit = 256 * 1024 * 1024
		}
	}
}

func SyncMode(s SyncModeType) Option {
	return func(o *Options) {
		o.SyncMode = s
	}
}

func MemoryLimit(bytes int64) Option {
	return func(o *Options) {
		o.MemoryLimit = bytes
	}
}

// Storage selects memory or disk storage for all database models.
func Storage(kind StorageKind) Option {
	return func(o *Options) {
		o.Storage = kind
		o.BackendFactory = nil
	}
}

// DefaultDisk makes disk storage the default for every persistent model.
func DefaultDisk() Option { return Storage(StorageDisk) }

// DefaultMemory makes RAM storage the default for every persistent model.
func DefaultMemory() Option { return Storage(StorageMemory) }

// ModelStorage overrides storage for a single model. When a model differs
// from the default, it is stored in a dedicated engine. Cross-engine updates
// commit independently and therefore are not one atomic ACID transaction.
func ModelStorage(model Model, kind StorageKind) Option {
	return func(o *Options) {
		if o.ModelStorage == nil {
			o.ModelStorage = make(map[Model]ModelStorageConfig)
		}
		o.ModelStorage[model] = ModelStorageConfig{Kind: kind}
	}
}

func KVStorage(kind StorageKind) Option       { return ModelStorage(ModelKV, kind) }
func DocumentStorage(kind StorageKind) Option { return ModelStorage(ModelDocument, kind) }
func QueueStorage(kind StorageKind) Option    { return ModelStorage(ModelQueue, kind) }

// ModelCustomStorage supplies a custom backend factory for one model.
func ModelCustomStorage(model Model, factory BackendFactory) Option {
	return func(o *Options) {
		if o.ModelStorage == nil {
			o.ModelStorage = make(map[Model]ModelStorageConfig)
		}
		o.ModelStorage[model] = ModelStorageConfig{Factory: factory}
	}
}

func KVCustomStorage(factory BackendFactory) Option {
	return ModelCustomStorage(ModelKV, factory)
}

func DocumentCustomStorage(factory BackendFactory) Option {
	return ModelCustomStorage(ModelDocument, factory)
}

func QueueCustomStorage(factory BackendFactory) Option {
	return ModelCustomStorage(ModelQueue, factory)
}

// CustomStorage configures an application-provided backend. The factory is
// called once for the main database and once for its WAL.
func CustomStorage(factory BackendFactory) Option {
	return func(o *Options) {
		o.BackendFactory = factory
	}
}

func openBackend(cfg Options, path string, wal bool) (file.Backend, error) {
	if cfg.BackendFactory != nil {
		backend, err := cfg.BackendFactory(path, cfg.ReadOnly, wal)
		if err != nil {
			return nil, err
		}
		if backend == nil {
			return nil, fmt.Errorf("coco: custom storage factory returned nil backend")
		}
		return backend, nil
	}

	switch cfg.Storage {
	case StorageMemory:
		return file.NewMemoryBackend(), nil
	case StorageDisk:
		if path == "" || path == ":memory:" {
			return nil, fmt.Errorf("coco: disk storage requires a database path")
		}
		return file.OpenOSBackend(path, cfg.ReadOnly)
	case StorageAuto:
		if path == "" || path == ":memory:" {
			return file.NewMemoryBackend(), nil
		}
		return file.OpenOSBackend(path, cfg.ReadOnly)
	default:
		return nil, fmt.Errorf("coco: unknown storage kind %d", cfg.Storage)
	}
}

func ReadOnly() Option {
	return func(o *Options) {
		o.ReadOnly = true
	}
}

func EncryptionKey(key []byte) Option {
	return func(o *Options) {
		o.EncryptionKey = key
	}
}

func EncryptionKeyID(id string) Option {
	return func(o *Options) {
		o.KeyID = id
	}
}

func Background(enabled bool) Option {
	return func(o *Options) {
		o.Background = enabled
	}
}

// EnableStudio enables the built-in Admin Studio web interface.
func EnableStudio(addr ...string) Option {
	return func(o *Options) {
		o.EnableStudio = true
		if len(addr) > 0 && addr[0] != "" {
			o.StudioAddr = addr[0]
		}
	}
}
