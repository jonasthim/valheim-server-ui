package events

import (
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestBus_Filtering(t *testing.T) {
	b := NewBus()
	main := b.Subscribe("main")
	defer main.Close()
	all := b.Subscribe("")
	defer all.Close()

	b.Publish(domain.Event{Name: "x", InstanceID: "main"})
	b.Publish(domain.Event{Name: "x", InstanceID: "other"})
	b.Publish(domain.Event{Name: "x", InstanceID: ""}) // global

	// The per-instance subscriber sees its own instance and global events, not
	// another instance's.
	got := drain(main.C)
	if len(got) != 2 {
		t.Fatalf("per-instance sub got %d events, want 2 (own + global)", len(got))
	}
	for _, ev := range got {
		if ev.InstanceID == "other" {
			t.Fatal("per-instance sub must not receive another instance's events")
		}
	}

	// The catch-all subscriber sees everything.
	if n := len(drain(all.C)); n != 3 {
		t.Fatalf("catch-all sub got %d events, want 3", n)
	}
}

func TestBus_OverflowDropsOldest(t *testing.T) {
	b := NewBus()
	s := b.Subscribe("") // never drained until the end
	defer s.Close()

	const n = subscriberBuffer + 50
	for i := 1; i <= n; i++ {
		b.Publish(domain.Event{Name: "x", Data: i})
	}

	got := drain(s.C)
	if len(got) != subscriberBuffer {
		t.Fatalf("buffer held %d events, want the cap %d", len(got), subscriberBuffer)
	}
	// Drop-oldest keeps the newest: the last published event must be present
	// and the first retained must be n-subscriberBuffer+1, not 1.
	if first := got[0].Data.(int); first != n-subscriberBuffer+1 {
		t.Fatalf("oldest retained = %d, want %d (oldest should have been dropped)", first, n-subscriberBuffer+1)
	}
	if last := got[len(got)-1].Data.(int); last != n {
		t.Fatalf("newest retained = %d, want %d", last, n)
	}
}

func TestBus_ConcurrentPublishAndClose(t *testing.T) {
	b := NewBus()
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Publishers.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.Publish(domain.Event{Name: "x", InstanceID: "main"})
				}
			}
		}()
	}
	// Subscribers that churn: subscribe, read a little, close — exercising the
	// RLock(publish)/Lock(close) discipline that guards send-on-closed-channel.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					sub := b.Subscribe("main")
					select {
					case <-sub.C:
					default:
					}
					sub.Close()
					sub.Close() // idempotent
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	if c := b.SubscriberCount(); c != 0 {
		t.Fatalf("all subscriptions closed, want 0 subscribers, got %d", c)
	}
}

// drain reads everything currently buffered on c without blocking.
func drain(c <-chan domain.Event) []domain.Event {
	var out []domain.Event
	for {
		select {
		case ev := <-c:
			out = append(out, ev)
		default:
			return out
		}
	}
}
