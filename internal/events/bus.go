// Package events is the in-process publish/subscribe bus that feeds the SSE
// endpoint. Publishing never blocks: slow subscribers drop their oldest events.
package events

import (
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

const subscriberBuffer = 256

type Subscription struct {
	C          <-chan domain.Event
	ch         chan domain.Event
	instanceID string // "" = all
	bus        *Bus
	id         uint64
}

// Close unsubscribes. Safe to call more than once.
func (s *Subscription) Close() { s.bus.unsubscribe(s.id) }

type Bus struct {
	mu   sync.RWMutex
	subs map[uint64]*Subscription
	next uint64
}

func NewBus() *Bus { return &Bus{subs: map[uint64]*Subscription{}} }

// Subscribe returns a subscription receiving every event, or only events for
// instanceID plus global events when instanceID is non-empty.
func (b *Bus) Subscribe(instanceID string) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	ch := make(chan domain.Event, subscriberBuffer)
	s := &Subscription{C: ch, ch: ch, instanceID: instanceID, bus: b, id: b.next}
	b.subs[s.id] = s
	return s
}

func (b *Bus) unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(s.ch)
	}
}

// Publish fans the event out. Implements domain.Publisher.
func (b *Bus) Publish(ev domain.Event) {
	if ev.TS.IsZero() {
		ev.TS = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if s.instanceID != "" && ev.InstanceID != "" && s.instanceID != ev.InstanceID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			// drop oldest, then enqueue
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- ev:
			default:
			}
		}
	}
}

// SubscriberCount is for tests and /system diagnostics.
func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
