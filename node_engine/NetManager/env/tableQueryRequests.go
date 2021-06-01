package env

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
)

type tableQueryResponse struct {
	AppName      string     `json:"app_name"`
	InstanceList []instance `json:"instance_list"`
}

type instance struct {
	InstanceNumber int    `json:"instance_number"`
	NamespaceIp    string `json:"namespace_ip"`
	HostIp         string `json:"host_ip"`
	HostPort       int    `json:"host_port"`
	ServiceIp      []sip  `json:"service_ip"`
}

type sip struct {
	Type    string `json:"IpType"`
	Address string `json:"Address"`
}

func tableQueryByIP(addr string, port string, ip string) ([]TableEntry, bool) {
	queryAddress := addr + ":" + port + "/api/job/ip/" + ip + "/instances"
	log.Println("[TABLE QUERY]", queryAddress, " ip:", ip)

	resp, err := http.Get(queryAddress)
	if err != nil {
		log.Println(err)
		return nil, false
	}

	reqBody, _ := ioutil.ReadAll(resp.Body)
	log.Println("[TABLE QUERY RESPONSE]", reqBody)

	var responseStruct tableQueryResponse
	err = json.Unmarshal(reqBody, &responseStruct)
	if err != nil {
		log.Println(err)
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
