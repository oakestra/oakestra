package events

import "sync"

type EventManager interface {
	Emit(event Event)
	Register(eventType EventType) chan Event
}

type Events struct {
	//map of event target to event kind
	eventTableQueryChannelQueue map[EventType]*[]chan Event
}

type Event struct {
	EventType    EventType
	EventMessage string
}

type EventType int

const (
	TableQuery EventType = iota
)

/* ------------- singleton instance ------- */
var once sync.Once
var (
	eventInstance EventManager
)

/* ------------------------------------------*/

func GetInstance() EventManager {
	once.Do(func() {
		eventInstance = &Events{
			eventTableQueryChannelQueue: make(map[EventType]*[]chan Event, 10),
		}
	})
	return eventInstance
}

func (e *Events) Emit(event Event) {
	switch event.EventType {
	case TableQuery:
		chanList := e.eventTableQueryChannelQueue[event.EventType]
		if chanList != nil {
			for _, channel := range *chanList {
				channel <- event
			}
		}
	}
}

func (e *Events) Register(eventType EventType) chan Event {
	eventChan := make(chan Event, 1)
	chanListPointer := e.eventTableQueryChannelQueue[eventType]
	if chanListPointer == nil {
		chanList := make([]chan Event, 0)
		chanListPointer = &chanList
	}
	chanList := append(*chanListPointer, eventChan)
	e.eventTableQueryChannelQueue[eventType] = &chanList
	return eventChan
}
