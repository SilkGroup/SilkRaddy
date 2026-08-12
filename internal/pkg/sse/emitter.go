package sse

import (
	"log/slog"
	"sync"
)

// Emitter manages a set of subscribers and broadcasts events to them.
//
// Sends are non-blocking: if a subscriber's channel is full the event is
// dropped for that subscriber rather than blocking every other consumer. The
// slow subscriber gets a diagnostic log line so operators can spot it.
type Emitter struct {
	subscribers sync.Map // key: chan *Event → value: bool (present)
	log         *slog.Logger
}

// NewEmitter creates and returns a new Emitter instance.
func NewEmitter() *Emitter {
	return &Emitter{}
}

// SetLogger attaches a logger used to report dropped events. Optional.
func (ee *Emitter) SetLogger(log *slog.Logger) {
	ee.log = log
}

// RegisterEvent broadcasts an event to every subscriber. If a subscriber's
// channel is full the event is dropped for that subscriber only.
func (ee *Emitter) RegisterEvent(name, data string) {
	event := NewEvent(name, data)
	ee.subscribers.Range(func(key, value any) bool {
		eventChan := key.(chan *Event)
		select {
		case eventChan <- event:
		default:
			if ee.log != nil {
				ee.log.Warn("sse: dropped event to slow subscriber", "event", name)
			}
		}
		return true
	})
}

// CountSubscribers returns the number of currently active subscribers.
func (ee *Emitter) CountSubscribers() int {
	count := 0
	ee.subscribers.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// Subscribe adds a subscriber channel to receive events. The channel should
// be buffered (see http handler) so the non-blocking send in RegisterEvent
// has somewhere to land.
func (ee *Emitter) Subscribe(eventChan chan *Event) {
	ee.subscribers.Store(eventChan, true)
}

// Unsubscribe removes a previously added subscriber channel.
func (ee *Emitter) Unsubscribe(eventChan chan *Event) {
	ee.subscribers.Delete(eventChan)
}
