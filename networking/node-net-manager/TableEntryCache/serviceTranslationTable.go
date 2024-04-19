package TableEntryCache

import (
	"NetManager/logger"
	"errors"
	"log"
	"net"
	"regexp"
	"sync"
)

type TableEntry struct {
	JobName          string      `json:"job_name"`
	Appname          string      `json:"appname"`
	Appns            string      `json:"appns"`
	Servicename      string      `json:"servicename"`
	Servicenamespace string      `json:"servicenamespace"`
	Instancenumber   int         `json:"instancenumber"`
	Cluster          int         `json:"cluster"`
	Nodeip           net.IP      `json:"nodeip"`
	Nodeport         int         `json:"nodeport"`
	Nsip             net.IP      `json:"nsip"`
	Nsipv6           net.IP      `json:"nsipv6"`
	ServiceIP        []ServiceIP `json:"serviceIP"`
}

type ServiceIpType int

const (
	InstanceNumber ServiceIpType = iota
	Closest        ServiceIpType = iota
	RoundRobin     ServiceIpType = iota
)

type ServiceIP struct {
	IpType     ServiceIpType `json:"ip_type"`
	Address    net.IP        `json:"address"`
	Address_v6 net.IP        `json:"address_v6"`
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
	// TODO cleanup of old entry every X seconds
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

// remove by Namespace IP, which can be either in IPv4 or IPv6 format
func (t *TableManager) RemoveByNsip(nsip net.IP) error {
	logger.DebugLogger().Printf("Remove by Nsip tableManager: %v", t)

	t.rwlock.Lock()
	defer t.rwlock.Unlock()

	found := -1
	// this will need to be optimised for IPv6, since that will be hell performance wise
	for i, tableElement := range t.translationTable {
		if tableElement.Nsip.Equal(nsip) || tableElement.Nsipv6.Equal(nsip) {
			found = i
			break
		}
	}

	return t.removeByIndex(found)
}

func (t *TableManager) RemoveByJobName(jobname string) error {
	t.rwlock.Lock()
	defer t.rwlock.Unlock()

	elems := len(t.translationTable)
	for i := 0; i < elems; i++ {
		if t.translationTable[i].JobName == jobname {
			err := t.removeByIndex(i)
			if err != nil {
				return err
			}
			elems = elems - 1
			i = i - 1
		}
	}
	return nil
}

func (t *TableManager) removeByIndex(index int) error {
	if index > -1 {
		logger.DebugLogger().Printf("Removing from TableManager: %v", t.translationTable[index])
		t.translationTable[index] = t.translationTable[len(t.translationTable)-1]
		t.translationTable = t.translationTable[:len(t.translationTable)-1]
		return nil
	}
	return errors.New("entry not found")
}

func (t *TableManager) SearchByServiceIP(ip net.IP) []TableEntry {
	// log.Println("Table research, table length: ", len(t.translationTable))
	// log.Println(t.translationTable)
	result := make([]TableEntry, 0)
	t.rwlock.Lock()
	defer t.rwlock.Unlock()
	for _, tableElement := range t.translationTable {
		for _, elemip := range tableElement.ServiceIP {
			if elemip.Address.Equal(ip) || elemip.Address_v6.Equal(ip) {
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
		if tableElement.Nsip.Equal(ip) || tableElement.Nsipv6.Equal(ip) {
			returnEntry := tableElement
			return returnEntry, true
		}
	}
	return TableEntry{}, false
}

func (t *TableManager) SearchByJobName(jobname string) []TableEntry {
	t.rwlock.Lock()
	defer t.rwlock.Unlock()
	results := make([]TableEntry, 0)
	for _, tableElement := range t.translationTable {
		if tableElement.JobName == jobname {
			results = append(results, tableElement)
		}
	}
	return results
}

// Sanity check for Appname and namespace
// 0<len(Appname)<11
// 0<len(Appns)<11
// 0<len(Servicename)<11
// 0<len(Servicenamespace)<11
// Instancenumber>0
// Cluster>0
// Nodeip != nil
// Nsip != nil
// Nsipv6 != nil
// len(entry.ServiceIP)>0
func (t *TableManager) isValid(entry TableEntry) bool {
	r, _ := regexp.Compile("^[a-zA-Z0-9]{1,30}$")

	if !r.MatchString(entry.Appname) {
		log.Println("TranslationTable: Invalid Entry, wrong appname:", entry.Appname)
		return false
	}
	if !r.MatchString(entry.Appns) {
		log.Println("TranslationTable: Invalid Entry, wrong appns:", entry.Appns)
		return false
	}
	if !r.MatchString(entry.Servicename) {
		log.Println("TranslationTable: Invalid Entry, wrong servicename:", entry.Servicename)
		return false
	}
	if !r.MatchString(entry.Servicenamespace) {
		log.Println("TranslationTable: Invalid Entry, wrong servicens:", entry.Servicenamespace)
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
	if entry.Nsipv6 == nil {
		log.Println("TranslationTable: Invalid Entry, wrong nsipv6")
		return false
	}
	if len(entry.ServiceIP) < 1 {
		log.Println("TranslationTable: Invalid Entry, wrong serviceip")
		return false
	}
	return true
}

func IsNamespaceStillValid(nsip net.IP, table *[]TableEntry) bool {
	for _, entry := range *table {
		if entry.Nsip.Equal(nsip) || entry.Nsipv6.Equal(nsip) {
			return true
		}
	}
	return false
}
