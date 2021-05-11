package proxy

import (
	"net"
	"sync"
	"time"
)

type ConversionEntry struct {
	srcip        net.IP
	dstip        net.IP
	dstServiceIp net.IP
	srcport      int
	dstport      int
}

type ConversionList struct {
	nextEntry      int
	lastUsed       int64
	conversionList []ConversionEntry
}

type ProxyCache struct {
	cache                 map[string]ConversionList //--> map[srcip.string]conversionlist
	conversionListMaxSize int
	rwlock                sync.RWMutex
}

func NewProxyCache() ProxyCache {
	return ProxyCache{
		cache:                 make(map[string]ConversionList),
		conversionListMaxSize: 10,
		rwlock:                sync.RWMutex{},
	}
	//TODO: Start cleanup procedure each X seconds
}

// Retrieve proxy cache entry based on source ip and source port and destination ServiceIP
func (cache *ProxyCache) RetrieveByServiceIP(srcip net.IP, srcport int, dstServiceIp net.IP) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem, exist := cache.cache[srcip.String()]
	if exist {
		elem.lastUsed = time.Now().Unix()
		for _, entry := range elem.conversionList {
			if entry.srcport == srcport && entry.dstServiceIp.Equal(dstServiceIp) {
				return entry, true
			}
		}
	}
	return ConversionEntry{}, false
}

// Retrieve proxy cache entry based on source ip and source port and destination ip
func (cache *ProxyCache) RetrieveByIp(srcip net.IP, dstport int, dstIP net.IP) (ConversionEntry, bool) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	elem, exist := cache.cache[srcip.String()]
	if exist {
		elem.lastUsed = time.Now().Unix()
		for _, entry := range elem.conversionList {
			if entry.dstport == dstport && entry.dstip.Equal(dstIP) {
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

	_, exist := cache.cache[entry.srcip.String()]
	if exist {
		cache.addExisting(entry)
	} else {
		cache.cache[entry.srcip.String()] = ConversionList{
			nextEntry:      0,
			lastUsed:       time.Now().Unix(),
			conversionList: make([]ConversionEntry, cache.conversionListMaxSize),
		}
		cache.addExisting(entry)
	}
}

func (cache *ProxyCache) addExisting(entry ConversionEntry) {
	elem, _ := cache.cache[entry.srcip.String()]
	elem.lastUsed = time.Now().Unix()
	alreadyExist := false
	alreadyExistPosition := 0
	//check if used port is already in cache
	for i, elementry := range elem.conversionList {
		if elementry.srcport == entry.srcport {
			alreadyExistPosition = i
			alreadyExist = true
		}
	}
	if alreadyExist {
		//if sourceport already in cache overwrite the cache entry
		elem.conversionList[alreadyExistPosition] = entry

	} else {
		//otherwise add a new cache entry in the next slot available
		elem.conversionList[elem.nextEntry] = entry
		elem.nextEntry = (elem.nextEntry + 1) % cache.conversionListMaxSize
	}
}

//TODO add tests
