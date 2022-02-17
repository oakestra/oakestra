package mqtt

import (
	"NetManager/events"
	"NetManager/utils"
	"encoding/json"
	"fmt"
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
}

type mqttInterestDeregisterRequest struct {
	Appname string `json:"appname"`
}

func (jut *jobUpdatesTimer) ConnectHandler(client mqtt.Client) {
	startSync.Lock()
	defer startSync.Unlock()
	if runningHandlers.Exists(jut.job) {
		log.Printf("Interest for job %s already registered", jut.job)
		return
	}
	jut.topic = "jobs/" + jut.job + "/updates_available"
	tqtoken := client.Subscribe(jut.topic, 1, jut.MessageHandler)
	tqtoken.Wait()
	log.Printf("Subscribed to job %s updates", jut.job)
	runningHandlers.Add(jut.job)
	jut.client = client
	go jut.StartSelfDestructTimeout()
}

func (jut *jobUpdatesTimer) ConnectionLostHandler(client mqtt.Client, err error) {
	//TODO: resiliency would be nice to have just in case :)
}

func (jut *jobUpdatesTimer) MessageHandler(client mqtt.Client, message mqtt.Message) {
	jut.env.RefreshServiceTable(jut.job)
}

func (jut *jobUpdatesTimer) StartSelfDestructTimeout() {
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
	cleanInterestTowardsJob(jut.job)
	jut.client.Unsubscribe(jut.topic)
	jut.client.Disconnect(0)
	eventManager.DeRegister(events.TableQuery, jut.job)
	runningHandlers.RemoveElem(jut.job)
}

func MqttRegisterInterest(jobName string, env jobEnvironmentManagerActions) {

	jobTimer := jobUpdatesTimer{
		eventManager: events.GetInstance(),
		job:          jobName,
		env:          env,
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", BrokerUrl, BrokerPort))
	opts.SetClientID(clientID)
	opts.SetUsername("")
	opts.SetPassword("")
	opts.OnConnect = jobTimer.ConnectHandler
	opts.OnConnectionLost = jobTimer.ConnectionLostHandler

	go runInterestClient(opts, jobName)
}

func runInterestClient(opts *mqtt.ClientOptions, jobName string) {
	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		cleanInterestTowardsJob(jobName)
		log.Printf("Error subscribing the topic for the job %s", jobName)
	}
}

func cleanInterestTowardsJob(jobName string) {
	request := mqttInterestDeregisterRequest{Appname: jobName}
	jsonreq, _ := json.Marshal(request)
	PublishToBroker("interest/remove", string(jsonreq))
}
