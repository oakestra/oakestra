package env

import (
	mqttifce "NetManager/mqtt"
	"errors"
	"log"
	"net"
	"strings"
)

/*
Asks the MQTT client for a table query and parses the result
*/
func tableQueryByIP(ip string) ([]TableEntry, error) {

	log.Println("[MQTT TABLE QUERY] sip:", ip)
	var mqttTablequery mqttifce.TablequeryMqttInterface = mqttifce.GetTableQueryRequestCacheInstance()

	responseStruct, err := mqttTablequery.TableQueryByIpRequestBlocking(ip)
	if err != nil {
		return nil, err
	}

	return responseParser(responseStruct)
}

/*
Asks the MQTT client for a table query and parses the result
*/
func tableQueryBySname(sname string) ([]TableEntry, error) {

	log.Println("[MQTT TABLE QUERY] sname:", sname)
	var mqttTablequery mqttifce.TablequeryMqttInterface = mqttifce.GetTableQueryRequestCacheInstance()

	responseStruct, err := mqttTablequery.TableQueryBySnameRequestBlocking(sname)
	if err != nil {
		return nil, err
	}

	return responseParser(responseStruct)
}

func responseParser(responseStruct mqttifce.TableQueryResponse) ([]TableEntry, error) {
	appCompleteName := strings.Split(responseStruct.AppName, ".")

	if len(appCompleteName) != 4 {
		return nil, errors.New("app complete name not of size 4")
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

	return result, nil
}
