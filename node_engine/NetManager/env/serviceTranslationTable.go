package env

import (
	"errors"
	"net"
	"sync"
)

type TableEntry struct {
	appname          string
	appns            string
	servicename      string
	servicenamespace string
	instancenumber   int
	cluster          int
	nodeip           net.IP
	nsip             net.IP
	serviceIP        []ServiceIP
}

type ServiceIpType int

const (
	InstanceNumber ServiceIpType = iota
	Closest        ServiceIpType = iota
	RoundRobin     ServiceIpType = iota
)

type ServiceIP struct {
	IpType  ServiceIpType
	address net.IP
}

type TableManager struct {
	translationTable []TableEntry
	rwlock           sync.RWMutex
}

func NewTableManager() TableManager {
	return TableManager{
		translationTable: make([]TableEntry, 0),
		rwlock:           sync.RWMutex{},
	}
	//TODO cleanup of old entry every X seconds
}

func (t *TableManager) Add(entry TableEntry) error {
	if t.isValid(entry) {
		t.rwlock.Lock()
		defer t.rwlock.Unlock()
		t.translationTable = append(t.translationTable, entry)
		return nil
	}
	return errors.New("InvalidEntry")
}

func (t *TableManager) SearchByServiceIP(ip net.IP) []TableEntry {
	result := make([]TableEntry, 0)
	t.rwlock.Lock()
	defer t.rwlock.Unlock()
	for _, tableElement := range t.translationTable {
		for _, elemip := range tableElement.serviceIP {
			if elemip.address.Equal(ip) {
				returnEntry := tableElement
				result = append(result, returnEntry)
			}
		}
	}
	return result
}

//Sanity chceck for appname and namespace
// 0<len(appname)<11
// 0<len(appns)<11
// 0<len(servicename)<11
// 0<len(servicenamespace)<11
// instancenumber>0
// cluster>0
// nodeip != nil
// nsip != nil
// len(entry.serviceIP)>0
func (t *TableManager) isValid(entry TableEntry) bool {
	if l := len(entry.appname); l < 1 || l > 10 {
		return false
	}
	if l := len(entry.appns); l < 1 || l > 10 {
		return false
	}
	if l := len(entry.servicename); l < 1 || l > 10 {
		return false
	}
	if l := len(entry.servicenamespace); l < 1 || l > 10 {
		return false
	}
	if entry.instancenumber < 0 {
		return false
	}
	if entry.cluster < 0 {
		return false
	}
	if entry.nodeip == nil {
		return false
	}
	if entry.nsip == nil {
		return false
	}
	if len(entry.serviceIP) < 1 {
		return false
	}
	return true
}
