package events

import (
	"testing"
	"time"
)

func TestSimpleEventEmit(t *testing.T) {
	eventManager := GetInstance()
	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
	})
}

func TestSimpleEventRegistration(t *testing.T) {
	eventManager := GetInstance()
	_ = eventManager.Register(TableQuery)
}

func TestRegisterAndEmit(t *testing.T) {
	eventManager := GetInstance()
	done := make(chan bool, 1)

	go func(done chan bool) {
		eventManager := GetInstance()
		eventchannel := eventManager.Register(TableQuery)
		done <- true
		select {
		case result := <-eventchannel:
			if result.EventMessage != "test" {
				t.Error("Event Not received")
			}
		case <-time.After(1 * time.Second):
			t.Error("did not receive a response in time")
		}
		done <- true
	}(done)

	<-done

	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
	})

	<-done

}

func TestRegisterMultipleAndEmit(t *testing.T) {

}
