package cachemap

import (
	"sync" // for RWMutex
)

type CacheItem []byte

type CacheMap struct {
    sync.RWMutex
    m map[string]CacheItem
}

func New() *CacheMap {
	return &(CacheMap{ m: make(map[string]CacheItem) })
}

// TODO: LRU
