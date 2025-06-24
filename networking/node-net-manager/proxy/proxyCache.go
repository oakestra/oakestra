package proxy

import (
	"net"
	"sync"
	"time"
)

type ConversionEntry struct {
	srcip         net.IP
	dstip         net.IP
	dstServiceIp  net.IP
	srcInstanceIp net.IP
	srcport       int
	dstport       int
}

type ConversionList struct {
	nextEntry      int
	lastUsed       int64
	conversionList []ConversionEntry
}

type ProxyCache struct {
	//One position for each port number. Higher mem usage but lower cpu usage
	cache                 []ConversionList
	conversionListMaxSize int
	rwlock                sync.RWMutex
}

func NewProxyCache() ProxyCache {
	cache := ProxyCache{
		cache:                 make([]ConversionList, 65535),
		conversionListMaxSize: 10,
		rwlock:                sync.RWMutex{},
	}
	cache.runEvictionJob(30*time.Second, 1*time.Minute)
	return cache
}

// runEvictionJob starts a goroutine that periodically evicts old cache entries.
func (cache *ProxyCache) runEvictionJob(interval time.Duration, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			cache.evictOldEntries(timeout)
		}
	}()
}

// evictOldEntries removes entries that have not been used for the duration of the timeout.
func (cache *ProxyCache) evictOldEntries(timeout time.Duration) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	now := time.Now().Unix()
	timeoutSeconds := int64(timeout.Seconds())
	evictedCount := 0
	for i, elem := range cache.cache {
		// An entry is considered old if it has not been used for the timeout period.
		if elem.lastUsed != 0 && (now-elem.lastUsed) > timeoutSeconds {
			// To evict the entry, we just reset it to its zero value.
			cache.cache[i] = ConversionList{}
			evictedCount++
		}
	}
	if evictedCount > 0 {
		logger.InfoLogger().Printf("Evicted %d entries from cache", evictedCount)
	}
}

// RetrieveByServiceIP Retrieve proxy proxycache entry based on source ip and source port and destination ServiceIP
func (cache *ProxyCache) RetrieveByServiceIP(srcip net.IP, instanceIP net.IP, srcport int, dstServiceIp net.IP, dstport int) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem := &cache.cache[srcport]
	if elem.conversionList != nil {
		for _, cacheEntry := range elem.conversionList {
			if cacheEntry.dstport == dstport &&
				cacheEntry.dstServiceIp.Equal(dstServiceIp) &&
				cacheEntry.srcip.Equal(srcip) &&
				cacheEntry.srcInstanceIp.Equal(instanceIP) {
				elem.lastUsed = time.Now().Unix()
				logger.InfoLogger().Printf("Found cached flow: %v\nCurrent length of cacheList: %d", cacheEntry, len(elem.conversionList))
				return cacheEntry, true
			}
		}
	}
	return ConversionEntry{}, false
}

// RetrieveByInstanceIp Retrieve proxy proxycache entry based on source ip and source port and destination ip
func (cache *ProxyCache) RetrieveByInstanceIp(srcip net.IP, srcport int, dstport int) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem := &cache.cache[srcport]
	if elem.conversionList != nil {
		for _, entry := range elem.conversionList {
			if entry.dstport == dstport && entry.srcip.Equal(srcip) {
				elem.lastUsed = time.Now().Unix()
				return entry, true
			}
		}
	}
	return ConversionEntry{}, false
}

// Add new conversion entry, if srcpip && srcport already added the entry is updated
func (cache *ProxyCache) Add(entry ConversionEntry) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem := &cache.cache[entry.srcport]
	if elem.conversionList == nil || len(elem.conversionList) == 0 {
		elem.nextEntry = 0
		elem.conversionList = make([]ConversionEntry, cache.conversionListMaxSize)
	}

	cache.addToConversionList(entry)
}

func (cache *ProxyCache) addToConversionList(entry ConversionEntry) {
	elem := &cache.cache[entry.srcport]
	elem.lastUsed = time.Now().Unix()
	alreadyExist := false
	alreadyExistPosition := 0
	//check if used port is already in proxycache
	for i, elementry := range elem.conversionList {
		if elementry.dstport == entry.dstport {
			alreadyExistPosition = i
			alreadyExist = true
			break
		}
	}
	if alreadyExist {
		//if sourceport already in proxycache overwrite the proxycache entry
		elem.conversionList[alreadyExistPosition] = entry

	} else {
		//otherwise add a new proxycache entry in the next slot available
		elem.conversionList[elem.nextEntry] = entry
		elem.nextEntry = (elem.nextEntry + 1) % cache.conversionListMaxSize
	}
}
