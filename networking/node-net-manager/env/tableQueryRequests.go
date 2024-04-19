package env

import (
	"NetManager/TableEntryCache"
	mqttifce "NetManager/mqtt"
	"errors"
	"log"
	"net"
	"strings"
)

/*
Asks the MQTT client for a table query and parses the result
*/
func tableQueryByIP(ip net.IP, force_optional ...bool) ([]TableEntryCache.TableEntry, error) {
	log.Println("[MQTT TABLE QUERY] sip:", ip.String())
	var mqttTablequery mqttifce.TablequeryMqttInterface = mqttifce.GetTableQueryRequestCacheInstance()

	responseStruct, err := mqttTablequery.TableQueryByIpRequestBlocking(ip.String(), force_optional...)
	if err != nil {
		return nil, err
	}

	return responseParser(responseStruct)
}

/*
Asks the MQTT client for a table query and parses the result
*/
func tableQueryByJobName(jobname string, force_optional ...bool) ([]TableEntryCache.TableEntry, error) {

	log.Println("[MQTT TABLE QUERY] sname:", jobname)
	var mqttTablequery mqttifce.TablequeryMqttInterface = mqttifce.GetTableQueryRequestCacheInstance()

	responseStruct, err := mqttTablequery.TableQueryByJobNameRequestBlocking(jobname, force_optional...)
	if err != nil {
		return nil, err
	}

	return responseParser(responseStruct)
}

func responseParser(responseStruct mqttifce.TableQueryResponse) ([]TableEntryCache.TableEntry, error) {
	appCompleteName := strings.Split(responseStruct.JobName, ".")

	if len(appCompleteName) != 4 {
		return nil, errors.New("app complete name not of size 4")
	}

	result := make([]TableEntryCache.TableEntry, 0)

	for _, instance := range responseStruct.InstanceList {
		sipList := make([]TableEntryCache.ServiceIP, 0)

		for _, ip := range instance.ServiceIp {
			sipList = append(sipList, toServiceIP(ip.Type, ip.Address, ip.Address_v6))
		}

		entry := TableEntryCache.TableEntry{
			JobName:          responseStruct.JobName,
			Appname:          appCompleteName[0],
			Appns:            appCompleteName[1],
			Servicename:      appCompleteName[2],
			Servicenamespace: appCompleteName[3],
			Instancenumber:   instance.InstanceNumber,
			Cluster:          0,
			Nodeip:           net.ParseIP(instance.HostIp),
			Nodeport:         instance.HostPort,
			Nsip:             net.ParseIP(instance.NamespaceIp),
			Nsipv6:           net.ParseIP(instance.NamespaceIpv6),
			ServiceIP:        sipList,
		}

		result = append(result, entry)
	}

	return result, nil
}

func toServiceIP(Type string, Addr string, Addr_v6 string) TableEntryCache.ServiceIP {
	ip := TableEntryCache.ServiceIP{
		IpType:     0,
		Address:    net.ParseIP(Addr),
		Address_v6: net.ParseIP(Addr_v6),
	}

	if Type == "RR" {
		ip.IpType = TableEntryCache.RoundRobin
	}
	if Type == "Closest" {
		ip.IpType = TableEntryCache.Closest
	}
	if Type == "InstanceNumber" {
		ip.IpType = TableEntryCache.InstanceNumber
	}

	return ip
}
