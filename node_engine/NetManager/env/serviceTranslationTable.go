package env

import (
	"errors"
	"log"
	"net"
	"sync"
)

type TableEntry struct {
	Appname          string
	Appns            string
	Servicename      string
	Servicenamespace string
	Instancenumber   int
	Cluster          int
	Nodeip           net.IP
	Nodeport         int
	Nsip             net.IP
	ServiceIP        []ServiceIP
}

type ServiceIpType int

const (
	InstanceNumber ServiceIpType = iota
	Closest        ServiceIpType = iota
	RoundRobin     ServiceIpType = iota
)

type ServiceIP struct {
	IpType  ServiceIpType
	Address net.IP
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
	log.Println("Table research, table length: ", len(t.translationTable))
	result := make([]TableEntry, 0)
	t.rwlock.Lock()
	defer t.rwlock.Unlock()
	for _, tableElement := range t.translationTable {
		for _, elemip := range tableElement.ServiceIP {
			if elemip.Address.Equal(ip) {
				returnEntry := tableElement
				result = append(result, returnEntry)
			}
		}
	}
	return result
}

func (t *TableManager) SearchByNsIP(ip net.IP) (TableEntry, bool) {
	t.rwlock.Lock()
	defer t.rwlock.Unlock()
	for _, tableElement := range t.translationTable {
		if tableElement.Nsip.Equal(ip) {
			returnEntry := tableElement
			return returnEntry, true
		}
	}
	return TableEntry{}, false
}

//Sanity chceck for Appname and namespace
// 0<len(Appname)<11
// 0<len(Appns)<11
// 0<len(Servicename)<11
// 0<len(Servicenamespace)<11
// Instancenumber>0
// Cluster>0
// Nodeip != nil
// Nsip != nil
// len(entry.ServiceIP)>0
func (t *TableManager) isValid(entry TableEntry) bool {
	if l := len(entry.Appname); l < 1 || l > 10 {
		log.Println("TranslationTable: Invalid Entry, wrong appname")
		return false
	}
	if l := len(entry.Appns); l < 1 || l > 10 {
		log.Println("TranslationTable: Invalid Entry, wrong appns")
		return false
	}
	if l := len(entry.Servicename); l < 1 || l > 10 {
		log.Println("TranslationTable: Invalid Entry, wrong servicename")
		return false
	}
	if l := len(entry.Servicenamespace); l < 1 || l > 10 {
		log.Println("TranslationTable: Invalid Entry, wrong servicens")
		return false
	}
	if entry.Instancenumber < 0 {
		log.Println("TranslationTable: Invalid Entry, wrong instancenumber")
		return false
	}
	if entry.Cluster < 0 {
		log.Println("TranslationTable: Invalid Entry, wrong cluster")
		return false
	}
	if entry.Nodeip == nil {
		log.Println("TranslationTable: Invalid Entry, wrong nodeip")
		return false
	}
	if entry.Nsip == nil {
		log.Println("TranslationTable: Invalid Entry, wrong nsip")
		return false
	}
	if len(entry.ServiceIP) < 1 {
		log.Println("TranslationTable: Invalid Entry, wrong serviceip")
		return false
	}
	return true
}
