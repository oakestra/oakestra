package events

import (
	"testing"
	"time"
)

func handlers(done chan bool, job string) {
	eventManager := GetInstance()
	eventchannel, _ := eventManager.Register(TableQuery, job)
	done <- true
	select {
	case _ = <-eventchannel:
		done <- true
	case <-time.After(1 * time.Second):
		done <- false
	}
}

func TestSimpleEventEmit(t *testing.T) {
	eventManager := GetInstance()
	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
		EventTarget:  "regtest",
	})
}

func TestSimpleEventRegistration(t *testing.T) {
	eventManager := GetInstance()
	_, err := eventManager.Register(TableQuery, "regtest")
	if err != nil {
		t.Error("Invalid registration result")
	}
}

func TestRegisterAndEmit(t *testing.T) {
	eventManager := GetInstance()
	done := make(chan bool, 1)

	go handlers(done, "job0")

	<-done

	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
		EventTarget:  "job0",
	})

	res := <-done
	if !res {
		t.Error("Event not received")
	}
}

func TestRegisterAndEmitWrongEvent(t *testing.T) {
	eventManager := GetInstance()
	done := make(chan bool, 1)

	go handlers(done, "job1")

	<-done

	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
		EventTarget:  "job2",
	})

	res := <-done
	if res {
		t.Error("Received wrong event")
	}
}

func TestRegisterMultipleAndEmit(t *testing.T) {
	eventManager := GetInstance()
	done1 := make(chan bool, 1)
	done2 := make(chan bool, 1)

	go handlers(done1, "job2")
	go handlers(done2, "job3")

	<-done1
	<-done2

	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
		EventTarget:  "job2",
	})
	eventManager.Emit(Event{
		EventType:    TableQuery,
		EventMessage: "test",
		EventTarget:  "job3",
	})

	res1 := <-done1
	res2 := <-done2
	if !res1 {
		t.Error("Event not received")
	}
	if !res2 {
		t.Error("Event not received")
	}

}
