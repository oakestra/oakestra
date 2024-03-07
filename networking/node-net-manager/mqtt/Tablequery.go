package mqtt

import (
	"NetManager/logger"
	"encoding/json"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

/*---- Singleton cache instance  ----*/
var once sync.Once
var (
	tableQueryRequestCacheInstance TableQueryRequestCache
)

/*-----------------------------------*/

/*----- Mqtt Table query cache classes and interfaces -----*/
type TablequeryMqttInterface interface {
	TableQueryByIpRequestBlocking(sip string, force_optional ...bool) (TableQueryResponse, error)
	TableQueryByJobNameRequestBlocking(sname string, force_optional ...bool) (TableQueryResponse, error)
}

type TableQueryRequestCache struct {
	siprequests map[string]*[]chan TableQueryResponse
	requestadd  sync.RWMutex
}

/*---------------------------------------------------------*/

/*------------------- Mqtt responses and requests ----------------------*/
type TableQueryResponse struct {
	JobName      string            `json:"app_name"`
	InstanceList []ServiceInstance `json:"instance_list"`
	QueryKey     string            `json:"query_key"`
}

type ServiceInstance struct {
	InstanceNumber int    `json:"instance_number"`
	NamespaceIp    string `json:"namespace_ip"`
	NamespaceIpv6  string `json:"namespace_ip_v6"`
	HostIp         string `json:"host_ip"`
	HostPort       int    `json:"host_port"`
	ServiceIp      []Sip  `json:"service_ip"`
}

type Sip struct {
	Type       string `json:"IpType"`
	Address    string `json:"Address"`
	Address_v6 string `json:"Address_v6"`
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
The call is blocking and awaits the response for a maximum of 10 seconds
set the force value to force the table query even in the event of interest already registered. Used in case of incoming updates notification.
*/
func (cache *TableQueryRequestCache) tableQueryRequestBlocking(sip string, sname string, force_optional ...bool) (TableQueryResponse, error) {
	reqname := sip + sname

	force := false
	if len(force_optional) > 0 {
		force = force_optional[0]
	}

	// If the worker node already registered an interest towards this route, avoid new requests.
	if MqttIsInterestRegistered(reqname) && !force {
		return TableQueryResponse{}, errors.New("interest already registered")
	}

	responseChannel := make(chan TableQueryResponse, 10)
	var updatedRequests []chan TableQueryResponse

	//appending response channel used by the Mqtt handler
	cache.requestadd.Lock()
	siprequests := cache.siprequests[reqname]
	if siprequests != nil {
		cache.requestadd.Unlock()
		return TableQueryResponse{}, errors.New("Table query already happening for this address")
		//updatedRequests = *siprequests
	}
	updatedRequests = append(updatedRequests, responseChannel)
	cache.siprequests[reqname] = &updatedRequests
	cache.requestadd.Unlock()

	//publishing mqtt message
	jsonreq, _ := json.Marshal(tableQueryRequest{
		Sname: sname,
		Sip:   sip,
	})
	_ = GetNetMqttClient().PublishToBroker("tablequery/request", string(jsonreq))

	//waiting for maximum 5 seconds the mqtt handler to receive a response. Otherwise fail the tableQuery.
	log.Printf("waiting for table query %s", reqname)
	select {
	case result := <-responseChannel:
		return result, nil
	case <-time.After(5 * time.Second):
		logger.ErrorLogger().Printf("TIMEOUT - Table query without response, quitting goroutine")
	}

	return TableQueryResponse{}, net.UnknownNetworkError("Mqtt Timeout")
}

/*
Perform a table query by ServiceIp to the cluster manager
The call is blocking and awaits the response for a maximum of 5 seconds
*/
func (cache *TableQueryRequestCache) TableQueryByIpRequestBlocking(sip string, force_optional ...bool) (TableQueryResponse, error) {
	return cache.tableQueryRequestBlocking(sip, "", force_optional...)
}

/*
Perform a table query by ServiceName to the cluster manager
The call is blocking and awaits the response for a maximum of 5 seconds
*/
func (cache *TableQueryRequestCache) TableQueryByJobNameRequestBlocking(jobname string, force_optional ...bool) (TableQueryResponse, error) {
	return cache.tableQueryRequestBlocking("", jobname, force_optional...)
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
	querykeys = append(querykeys, responseStruct.JobName)
	querykeys = append(querykeys, responseStruct.QueryKey)
	for _, instance := range responseStruct.InstanceList {
		for _, sip := range instance.ServiceIp {
			querykeys = append(querykeys, sip.Address)
			querykeys = append(querykeys, sip.Address_v6)
		}
	}

	//notify hanging channels for each query key
	for _, key := range querykeys {
		cache.requestadd.Lock()
		channelList := cache.siprequests[key]
		if channelList != nil {
			for _, channel := range *channelList {
				logger.DebugLogger().Printf("TableQuery response - notifying a channel regarding %s", key)
				channel <- responseStruct
				logger.DebugLogger().Printf("TableQuery response - channel notified")
			}
		}
		cache.siprequests[key] = nil
		cache.requestadd.Unlock()
	}
}
