package cachemap

import (
	"sync" // for RWMutex
)

type CacheItem []byte

type CacheMap struct {
    mutex sync.RWMutex
    m map[string]CacheItem
}

func New() *CacheMap {
	return &(CacheMap{ m: make(map[string]CacheItem) })
}

func (cm *CacheMap)Get(key string) (CacheItem, bool) {
	cm.mutex.RLock()
	val, exists := cm.m[key]
	cm.mutex.RUnlock()

	return val, exists
}

func (cm *CacheMap)Set(key string, val CacheItem) {
	cm.mutex.Lock()
	cm.m[key] = val
	cm.mutex.Unlock()
}

func (cm *CacheMap)Delete(key string) bool {
	cm.mutex.Lock()
	_, exists := cm.m[key]
	delete(cm.m, key)
	cm.mutex.Unlock()

	return exists
}

// TODO: LRU
