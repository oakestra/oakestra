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

var runningHandlers = utils.NewStringSlice()
var startSync sync.Mutex

type jobUpdatesTimer struct {
	eventManager events.EventManager
	job          string
	topic        string
	client       mqtt.Client
	env          jobEnvironmentManagerActions
}

type jobEnvironmentManagerActions interface {
	RefreshServiceTable(sname string)
	RemoveServiceEntries(sname string)
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
		If any worker still requires this job, reset timer. If in 5 minutes nobody needs this service, de-register the interest.
	*/
	log.Printf("self destruction timeout started for job %s", jut.job)
	eventManager := events.GetInstance()
	eventChan, _ := eventManager.Register(events.TableQuery, jut.job)
	for true {
		select {
		case <-eventChan:
			//event received, reset timer
			continue
		case <-time.After(5 * time.Minute):
			//timeout job no longer required
			break
		}
	}
	startSync.Lock()
	defer startSync.Unlock()
	log.Printf("De-registering from %s", jut.job)
	cleanInterestTowardsJob(jut.job)
	jut.client.Unsubscribe(jut.topic)
	delete(TOPICS, jut.topic) //removing topic from the topic list in case of disconnection
	eventManager.DeRegister(events.TableQuery, jut.job)
	jut.env.RemoveServiceEntries(jut.job)
	runningHandlers.RemoveElem(jut.job)
}

func MqttRegisterInterest(jobName string, env jobEnvironmentManagerActions) {

	startSync.Lock()
	defer startSync.Unlock()
	if runningHandlers.Exists(jobName) {
		log.Printf("Interest for job %s already registered", jobName)
		return
	}

	jobTimer := jobUpdatesTimer{
		eventManager: events.GetInstance(),
		job:          jobName,
		env:          env,
	}

	jobTimer.topic = "jobs/" + jobName + "/updates_available"
	TOPICS[jobTimer.topic] = jobTimer.MessageHandler //adding the topic to the global topic list to be handled in case of disconnection
	tqtoken := mainMqttClient.Subscribe(jobTimer.topic, 1, jobTimer.MessageHandler)
	tqtoken.Wait()
	log.Printf("MQTT - Subscribed to %s ", jobTimer.topic)
	runningHandlers.Add(jobTimer.job)
	go jobTimer.startSelfDestructTimeout()
}

func cleanInterestTowardsJob(jobName string) {
	request := mqttInterestDeregisterRequest{Appname: jobName}
	jsonreq, _ := json.Marshal(request)
	PublishToBroker("interest/remove", string(jsonreq))
}
