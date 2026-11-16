package mqtt

import (
	"NetManager/events"
	"NetManager/utils"
	"encoding/json"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"sync"
	"time"
)

var runningHandlers = utils.NewSlice[string]()
var runningHandlersLock sync.RWMutex

type jobUpdatesTimer struct {
	job      string
	instance int
	topic    string
	client   *NetMqttClient
	env      jobEnvironmentManagerActions
}

type jobEnvironmentManagerActions interface {
	RefreshServiceTable(sname string)
	RemoveServiceEntries(sname string)
	IsServiceDeployed(fullSnameAndInstance string) bool
}

type mqttInterestDeregisterRequest struct {
	Appname string `json:"appname"`
}

func (jut *jobUpdatesTimer) MessageHandler(client mqtt.Client, message mqtt.Message) {
	log.Printf("Received job update regarding %s", message.Topic())
	go jut.env.RefreshServiceTable(jut.job)
}

func (jut *jobUpdatesTimer) startSelfDestructTimeout() {
	/*
		If any worker still requires this job, keep the interest alive. If it goes
		10 seconds without being used and nothing local still needs it, de-register.
	*/
	log.Printf("self destruction timeout started for job %s", jut.job)
	activity := events.GetOrCreate(jut.job)
	const idleTimeout = 10 * time.Second
	for {
		time.Sleep(idleTimeout)
		if activity.IdleFor() < idleTimeout {
			// used again while we were sleeping, keep the interest alive
			continue
		}
		if !jut.env.IsServiceDeployed(jut.job) {
			//timeout ----> job no longer required. Let's clear the interest
			log.Printf("De-registering from %s", jut.job)
			cleanInterestTowardsJob(jut.job)
			jut.client.DeRegisterTopic(jut.topic)
			runningHandlersLock.Lock()
			runningHandlers.RemoveElem(jut.job)
			runningHandlersLock.Unlock()
			events.Delete(jut.job)
			jut.env.RemoveServiceEntries(jut.job)
			return
		}
	}
}

// MqttRegisterInterest :
/* Register an interest in a route for 5 minutes.
If the route is not used for more than 5 minutes the interest is removed
If the instance number is provided, the interest is kept until that instance is deployed in the node */
func MqttRegisterInterest(jobName string, env jobEnvironmentManagerActions, instance ...int) {

	runningHandlersLock.Lock()
	defer runningHandlersLock.Unlock()
	if runningHandlers.Exists(jobName) {
		log.Printf("Interest for job %s already registered", jobName)
		return
	}

	instanceNumber := -1
	if len(instance) > 0 {
		instanceNumber = instance[0]
	}

	jobTimer := jobUpdatesTimer{
		job:      jobName,
		env:      env,
		client:   GetNetMqttClient(),
		instance: instanceNumber,
	}

	jobTimer.topic = "jobs/" + jobName + "/updates_available"
	GetNetMqttClient().RegisterTopic(jobTimer.topic, jobTimer.MessageHandler)
	log.Printf("MQTT - Subscribed to %s ", jobTimer.topic)
	runningHandlers.Add(jobTimer.job)
	go jobTimer.startSelfDestructTimeout()
}

func MqttIsInterestRegistered(jobName string) bool {
	runningHandlersLock.RLock()
	defer runningHandlersLock.RUnlock()
	if runningHandlers.Exists(jobName) {
		return true
	}
	return false
}

func cleanInterestTowardsJob(jobName string) {
	request := mqttInterestDeregisterRequest{Appname: jobName}
	jsonreq, _ := json.Marshal(request)
	_ = GetNetMqttClient().PublishToBroker("interest/remove", string(jsonreq))
}
