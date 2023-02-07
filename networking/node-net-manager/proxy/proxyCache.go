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
	return ProxyCache{
		cache:                 make([]ConversionList, 65535),
		conversionListMaxSize: 10,
		rwlock:                sync.RWMutex{},
	}
}

// RetrieveByServiceIP Retrieve proxy proxycache entry based on source ip and source port and destination ServiceIP
func (cache *ProxyCache) RetrieveByServiceIP(srcip net.IP, instanceIP net.IP, srcport int, dstServiceIp net.IP, dstport int) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem := cache.cache[srcport]
	elem.lastUsed = time.Now().Unix()
	if elem.conversionList != nil {
		for _, cacheEntry := range elem.conversionList {
			if cacheEntry.dstport == dstport &&
				cacheEntry.dstServiceIp.Equal(dstServiceIp) &&
				cacheEntry.srcip.Equal(srcip) &&
				cacheEntry.srcInstanceIp.Equal(instanceIP) {
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

	elem := cache.cache[srcport]
	elem.lastUsed = time.Now().Unix()
	if elem.conversionList != nil {
		for _, entry := range elem.conversionList {
			if entry.dstport == dstport && entry.srcip.Equal(srcip) {
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

	elem := cache.cache[entry.srcport]
	if elem.conversionList == nil || len(elem.conversionList) == 0 {
		elem.nextEntry = 0
		elem.conversionList = make([]ConversionEntry, cache.conversionListMaxSize)
	}
	cache.cache[entry.srcport] = elem

	cache.addToConversionList(entry)
}

func (cache *ProxyCache) addToConversionList(entry ConversionEntry) {
	elem := cache.cache[entry.srcport]
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
