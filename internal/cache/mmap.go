package cache

import (
	"os"
	"sync"

	mmap "github.com/edsrzf/mmap-go"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/personalmedia/cdn/internal/config"
)

type MappedFile struct {
	path    string
	data    mmap.MMap
	modUnix int64
	size    int64
	mime    string
}

var (
	mmapLRU   *lru.Cache[string, *MappedFile]
	mmapLRUMu sync.Mutex
)

func initMmap() {
	var err error
	mmapLRU, err = lru.NewWithEvict(config.App.MMapCacheCap, func(_ string, mf *MappedFile) {
		if mf != nil && mf.data != nil {
			_ = mf.data.Unmap()
		}
	})
	if err != nil {
		panic(err)
	}
}

func GetMappedFile(path, mimeType string) ([]byte, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}

	modUnix := info.ModTime().UTC().UnixNano()
	size := info.Size()

	mmapLRUMu.Lock()
	defer mmapLRUMu.Unlock()

	if mf, ok := mmapLRU.Get(path); ok {
		if mf != nil && mf.modUnix == modUnix && mf.size == size {
			return mf.data, true, nil
		}
		mmapLRU.Remove(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	mm, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		return nil, false, err
	}

	entry := &MappedFile{
		path:    path,
		data:    mm,
		modUnix: modUnix,
		size:    size,
		mime:    mimeType,
	}

	mmapLRU.Add(path, entry)

	return entry.data, true, nil
}

func InvalidateMappedFile(path string) {
	mmapLRUMu.Lock()
	defer mmapLRUMu.Unlock()
	mmapLRU.Remove(path)
}
