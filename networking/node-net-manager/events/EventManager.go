package events

import (
	"errors"
	"sync"
)

type EventManager interface {
	Emit(event Event)
	Register(eventType EventType, eventTarget string) (chan Event, error)
	DeRegister(eventType EventType, eventTarget string)
}

type Events struct {
	//map of event target to event kind
	eventTableQueryChannelQueue map[string]chan Event
}

type Event struct {
	EventType    EventType
	EventTarget  string
	EventMessage string
}

type EventType int

const (
	TableQuery EventType = iota
)

/* ------------- singleton instance ------- */
var once sync.Once
var rwlock sync.RWMutex
var (
	eventInstance EventManager
)

/* ------------------------------------------*/

func GetInstance() EventManager {
	once.Do(func() {
		eventInstance = &Events{
			eventTableQueryChannelQueue: make(map[string]chan Event, 0),
		}
	})
	return eventInstance
}

func (e *Events) Emit(event Event) {
	rwlock.RLock()
	defer rwlock.RUnlock()
	switch event.EventType {
	case TableQuery:
		channel := e.eventTableQueryChannelQueue[event.EventTarget]
		if channel != nil {
			//check channel buffer capacity to prevent blocking. If this is false, probably no receiver is active.
			if len(channel) < cap(channel) {
				channel <- event
			}
		}
	}
}

func (e *Events) Register(eventType EventType, eventTarget string) (chan Event, error) {
	rwlock.Lock()
	defer rwlock.Unlock()
	switch eventType {
	case TableQuery:
		channel := e.eventTableQueryChannelQueue[eventTarget]
		if channel == nil {
			channel = make(chan Event, 10)
		}
		e.eventTableQueryChannelQueue[eventTarget] = channel
		return channel, nil
	}
	return nil, errors.New("Invalid EventType")
}

func (e *Events) DeRegister(eventType EventType, eventTarget string) {
	rwlock.Lock()
	defer rwlock.Unlock()
	switch eventType {
	case TableQuery:
		channel := e.eventTableQueryChannelQueue[eventTarget]
		if channel != nil {
			e.eventTableQueryChannelQueue[eventTarget] = nil
			close(channel)
		}
	}
}
