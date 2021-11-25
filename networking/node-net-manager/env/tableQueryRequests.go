package env

import (
	mqttifce "NetManager/mqtt"
	"log"
	"net"
	"strings"
)

/*
Asks the MQTT client for a table query and parses the result
*/
func tableQueryByIP(addr string, port string, ip string) ([]TableEntry, bool) {

	log.Println("[MQTT TABLE QUERY] sip:", ip)
	var mqttTablequery mqttifce.TablequeryMqttInterface = mqttifce.GetTableQueryRequestCacheInstance()

	responseStruct, err := mqttTablequery.TableQueryByIpRequestBlocking(ip)
	if err != nil {
		return nil, false
	}

	appCompleteName := strings.Split(responseStruct.AppName, ".")

	if len(appCompleteName) != 4 {
		return nil, false
	}

	result := make([]TableEntry, 0)

	for _, instance := range responseStruct.InstanceList {
		sipList := make([]ServiceIP, 0)

		for _, ip := range instance.ServiceIp {
			sipList = append(sipList, ToServiceIP(ip.Type, ip.Address))
		}

		entry := TableEntry{
			Appname:          appCompleteName[0],
			Appns:            appCompleteName[1],
			Servicename:      appCompleteName[2],
			Servicenamespace: appCompleteName[3],
			Instancenumber:   instance.InstanceNumber,
			Cluster:          0,
			Nodeip:           net.ParseIP(instance.HostIp),
			Nodeport:         instance.HostPort,
			Nsip:             net.ParseIP(instance.NamespaceIp),
			ServiceIP:        sipList,
		}

		result = append(result, entry)
	}

	return result, true
}
