package mqtt

import (
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"net"
	"sync"
	"time"
)

/*---- Singleton cache instance  ----*/
var once sync.Once
var (
	tableQueryRequestCacheInstance TableQueryRequestCache
)

/*-----------------------------------*/

/*----- Mqtt Table query cache classes and interfaces -----*/
type TablequeryMqttInterface interface {
	TableQueryByIpRequestBlocking(sip string) (TableQueryResponse, error)
	TableQueryBySnameRequestBlocking(sname string) (TableQueryResponse, error)
}

type TableQueryRequestCache struct {
	siprequests map[string]*[]chan TableQueryResponse
	requestadd  sync.RWMutex
}

/*---------------------------------------------------------*/

/*------------------- Mqtt responses and requests ----------------------*/
type TableQueryResponse struct {
	AppName      string            `json:"app_name"`
	InstanceList []ServiceInstance `json:"instance_list"`
}

type ServiceInstance struct {
	InstanceNumber int    `json:"instance_number"`
	NamespaceIp    string `json:"namespace_ip"`
	HostIp         string `json:"host_ip"`
	HostPort       int    `json:"host_port"`
	ServiceIp      []Sip  `json:"service_ip"`
}

type Sip struct {
	Type    string `json:"IpType"`
	Address string `json:"Address"`
}

type tableQueryRequest struct {
	Sname string `json:"sname"`
	Sip   string `json:"sip"`
}

/*------------------------------------------------------*/

/*
	returns a singleton pointer to an instance instance of the TableQueryRequestCache class
*/
func GetTableQueryRequestCacheInstance() *TableQueryRequestCache {
	once.Do(func() { // <-- atomic, does not allow repeating

		tableQueryRequestCacheInstance = TableQueryRequestCache{
			siprequests: make(map[string]*[]chan TableQueryResponse),
			requestadd:  sync.RWMutex{},
		}

	})
	return &tableQueryRequestCacheInstance
}

/*
	Perform a table query by ServiceIp to the cluster manager
	The call is blocking and awaits the response for a maximum of 5 seconds
*/
func (cache *TableQueryRequestCache) tableQueryRequestBlocking(sip string, sname string) (TableQueryResponse, error) {
	reqname := sip + sname
	responseChannel := make(chan TableQueryResponse, 1)
	var updatedRequests []chan TableQueryResponse

	//appending response channel used by the Mqtt handler
	cache.requestadd.Lock()
	siprequests := cache.siprequests[reqname]
	if siprequests != nil {
		updatedRequests = *siprequests
	}
	updatedRequests = append(updatedRequests, responseChannel)
	cache.siprequests[reqname] = &updatedRequests
	cache.requestadd.Unlock()

	//publishing mqtt message
	jsonreq, _ := json.Marshal(tableQueryRequest{
		Sname: sname,
		Sip:   sip,
	})
	go PublishToBroker("tablequery/request", string(jsonreq))

	//waiting for maximum 5 seconds the mqtt handler to receive a response. Otherwise fail the tableQuery.
	log.Printf("waiting for table query %s", reqname)
	select {
	case result := <-responseChannel:
		return result, nil
	case <-time.After(5 * time.Second):
		log.Printf("TIMEOUT - Table query without response, quitting goroutine")
	}

	return TableQueryResponse{}, net.UnknownNetworkError("Mqtt Timeout")
}

/*
	Perform a table query by ServiceIp to the cluster manager
	The call is blocking and awaits the response for a maximum of 5 seconds
*/
func (cache *TableQueryRequestCache) TableQueryByIpRequestBlocking(sip string) (TableQueryResponse, error) {
	return cache.tableQueryRequestBlocking(sip, "")
}

/*
	Perform a table query by ServiceName to the cluster manager
	The call is blocking and awaits the response for a maximum of 5 seconds
*/
func (cache *TableQueryRequestCache) TableQueryBySnameRequestBlocking(sname string) (TableQueryResponse, error) {
	return cache.tableQueryRequestBlocking("", sname)
}

/*
	Handler used by the mqtt client to dispatch the table query result
*/
func (cache *TableQueryRequestCache) TablequeryResultMqttHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("MQTT - Received mqtt table query message: %s", msg.Payload())

	//response parsing
	payload := msg.Payload()
	var responseStruct TableQueryResponse
	err := json.Unmarshal(payload, &responseStruct)
	if err != nil {
		log.Println(err)
	}

	//extract sip and app names as query keys
	querykeys := make([]string, 0)
	querykeys = append(querykeys, responseStruct.AppName)
	for _, instance := range responseStruct.InstanceList {
		for _, sip := range instance.ServiceIp {
			querykeys = append(querykeys, sip.Address)
		}
	}

	//notify hanging channels for each query key
	for _, key := range querykeys {
		cache.requestadd.Lock()
		channelList := cache.siprequests[key]
		if channelList != nil {
			for _, channel := range *channelList {
				log.Printf("TableQuery response - notifying a channel regarding %s", key)
				channel <- responseStruct
			}
		}
		cache.siprequests[key] = nil
		cache.requestadd.Unlock()
	}
}
