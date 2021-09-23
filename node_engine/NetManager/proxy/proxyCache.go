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
	//todo map by address and not by destination port, this will cause troubles.
	cache                 map[int]ConversionList //--> map[dstport]conversionlist
	conversionListMaxSize int
	rwlock                sync.RWMutex
}

func NewProxyCache() ProxyCache {
	return ProxyCache{
		cache:                 make(map[int]ConversionList),
		conversionListMaxSize: 10,
		rwlock:                sync.RWMutex{},
	}
	//TODO: Start cleanup procedure each X seconds
}

// Retrieve proxy proxycache entry based on source ip and source port and destination ServiceIP
func (cache *ProxyCache) RetrieveByServiceIP(srcIP net.IP, srcport int, dstServiceIp net.IP, dstport int) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem, exist := cache.cache[srcport]
	if exist {
		elem.lastUsed = time.Now().Unix()
		for _, entry := range elem.conversionList {
			if entry.dstport == dstport && entry.dstServiceIp.Equal(dstServiceIp) && entry.srcip.Equal(srcIP) {
				return entry, true
			}
		}
	}
	return ConversionEntry{}, false
}

// Retrieve proxy proxycache entry based on source ip and source port and destination ip
func (cache *ProxyCache) RetrieveByInstanceIp(srcip net.IP, srcport int, dstport int) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem, exist := cache.cache[srcport]
	if exist {
		elem.lastUsed = time.Now().Unix()
		for _, entry := range elem.conversionList {
			if entry.dstport == dstport && entry.srcip.Equal(srcip) {
				return entry, true
			}
		}
	}
	return ConversionEntry{}, false
}

// Add add new conversion entry, if srcpip && srcport already added the entry is updated
func (cache *ProxyCache) Add(entry ConversionEntry) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	_, exist := cache.cache[entry.srcport]
	if exist {
		cache.addExisting(entry)
	} else {
		cache.cache[entry.srcport] = ConversionList{
			nextEntry:      0,
			lastUsed:       time.Now().Unix(),
			conversionList: make([]ConversionEntry, cache.conversionListMaxSize),
		}
		cache.addExisting(entry)
	}
}

func (cache *ProxyCache) addExisting(entry ConversionEntry) {
	elem, _ := cache.cache[entry.srcport]
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
