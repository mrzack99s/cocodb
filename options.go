package cocodb

import "time"

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

type Options struct {
	Profile       ProfileType
	SyncMode      SyncModeType
	MemoryLimit   int64
	ReadOnly      bool
	EncryptionKey []byte
	KeyID         string
	Background    bool
	CleanInterval time.Duration
	EnableStudio  bool
	StudioAddr    string
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
