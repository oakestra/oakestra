package proxy

import (
	"net"
	"sync"
)

type HostEntry struct {
	srcip net.IP
	host  net.UDPAddr
}

type HostCache struct {
	nextEntry      int
	maxentry       int
	rwlock         sync.RWMutex
	conversionList []HostEntry
}

func NewHostCache() HostCache {
	return HostCache{
		nextEntry:      0,
		maxentry:       20,
		rwlock:         sync.RWMutex{},
		conversionList: make([]HostEntry, 20),
	}
}

//Get entry if any
func (cache *HostCache) Get(srcIP net.IP) (HostEntry, bool) {
	for _, el := range cache.conversionList {
		if el.srcip.Equal(srcIP) {
			return el, true
		}
	}
	return HostEntry{}, false
}

// Add add new conversion entry, if srcpip && srcport already added the entry is updated
func (cache *HostCache) Add(entry HostEntry) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	alreadyExist := -1
	for i, elementry := range cache.conversionList {
		if elementry.srcip.Equal(entry.srcip) {
			alreadyExist = i
		}
	}
	if alreadyExist > -1 {
		//if sourceport already in proxycache overwrite the proxycache entry
		cache.conversionList[alreadyExist] = entry
	} else {
		//otherwise add a new proxycache entry in the next slot available
		cache.conversionList[cache.nextEntry] = entry
		cache.nextEntry = (cache.nextEntry + 1) % cache.maxentry
	}
}
