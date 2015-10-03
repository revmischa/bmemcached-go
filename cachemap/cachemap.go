package cachemap

// The actual storage of our objects. Safe for concurrent accesses.

// TODO: LRU + expiry so that memory doesn't grow forever


import (
	"sync" // for RWMutex
)

type Flags []byte

type CacheItem struct {
	flags Flags
	val   []byte
}

type CacheMap struct {
	mutex sync.RWMutex
	m     map[string]CacheItem
}

func New() *CacheMap {
	return &(CacheMap{m: make(map[string]CacheItem)})
}


/// Mediate access with a read/write mutex

func (cm *CacheMap) Get(key string) ([]byte, Flags, bool) {
	cm.mutex.RLock()
	val, exists := cm.m[key]
	cm.mutex.RUnlock()

	return val.val, val.flags, exists
}

func (cm *CacheMap) Set(key string, val []byte, flags Flags) {
	cm.mutex.Lock()
	cm.m[key] = CacheItem{flags: flags, val: val}
	cm.mutex.Unlock()
}

func (cm *CacheMap) Delete(key string) bool {
	cm.mutex.Lock()
	_, exists := cm.m[key]
	delete(cm.m, key)
	cm.mutex.Unlock()

	return exists
}
