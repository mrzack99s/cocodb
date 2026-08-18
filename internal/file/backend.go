package file

// Backend abstracts physical file I/O for persistent and in-memory databases.
type Backend interface {
	ReadAt(p []byte, off int64) (int, error)
	WriteAt(p []byte, off int64) (int, error)
	Sync() error
	Truncate(size int64) error
	Size() (int64, error)
	Close() error
}
