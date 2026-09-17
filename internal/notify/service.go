package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/metrics"
)

// cooldown bounds how often the same (channel, kind, instance) tuple sends,
// so a noisy source of events (e.g. a flapping instance) does not spam a
// channel.
const cooldown = 10 * time.Minute

// sendTimeout bounds one delivery attempt; Test and the event-driven path
// both use it.
const sendTimeout = 10 * time.Second

// defaultDiskLowPercent is used when NotifySettings.DiskLowPercent is 0.
const defaultDiskLowPercent = 10

// defaultDiskCheckInterval is how often the disk ticker samples free space.
const defaultDiskCheckInterval = 5 * time.Minute

// LogRepo persists notification delivery attempts (internal/db.NotificationLogRepo).
type LogRepo interface {
	Insert(ctx context.Context, entry domain.NotificationLogEntry) error
	List(ctx context.Context, limit int, before *time.Time) ([]domain.NotificationLogEntry, error)
}

// Service implements api.NotifyService: it watches the event bus, maps
// events to alerts (rules.go) and delivers them to every enabled, subscribed
// channel (sender.go), logging each attempt.
type Service struct {
	bus      *events.Bus
	settings func() domain.NotifySettings
	dataDir  string
	logRepo  LogRepo
	log      *slog.Logger

	sender Sender // set by WithSender in tests; overrides senderFor(type)
	now    func() time.Time

	// diskCheckInterval is a var (not a const) so tests can shrink it.
	diskCheckInterval time.Duration

	mu        sync.Mutex
	lastSent  map[string]time.Time            // "channelID|kind|instanceID" -> last send time
	downState map[string]domain.InstanceState // instanceID -> last seen instance.status state
	players   map[string]playersSnapshot      // instanceID -> online set
}

// Option configures a Service at construction time.
type Option func(*Service)

// WithSender overrides the sender used for every channel type (tests only).
func WithSender(sn Sender) Option {
	return func(s *Service) { s.sender = sn }
}

// WithClock overrides time.Now (tests only).
func WithClock(fn func() time.Time) Option {
	return func(s *Service) {
		if fn != nil {
			s.now = fn
		}
	}
}

// New builds the notification Service. Call Run to start watching the bus.
// settings is re-read on every event/tick so a settings change (channels
// added, disk threshold changed) takes effect without a restart, mirroring
// selfupdate.Checker/steam.UpdateChecker.
func New(bus *events.Bus, settings func() domain.NotifySettings, dataDir string, logRepo LogRepo, log *slog.Logger, opts ...Option) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		bus:               bus,
		settings:          settings,
		dataDir:           dataDir,
		logRepo:           logRepo,
		log:               log,
		now:               time.Now,
		diskCheckInterval: defaultDiskCheckInterval,
		lastSent:          map[string]time.Time{},
		downState:         map[string]domain.InstanceState{},
		players:           map[string]playersSnapshot{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run subscribes to every bus event and runs the disk-space ticker, until ctx
// is done. It never blocks the bus: each delivery runs in its own goroutine.
func (s *Service) Run(ctx context.Context) {
	if s.bus == nil {
		<-ctx.Done()
		return
	}
	sub := s.bus.Subscribe("")
	defer sub.Close()

	ticker := time.NewTicker(s.diskCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			for _, m := range s.messagesFor(ev) {
				s.dispatch(ctx, m)
			}
		case <-ticker.C:
			s.checkDisk(ctx)
		}
	}
}

// messagesFor runs the event through alertsFor, threading the per-instance
// players snapshot, and applies the "AlertDown only on a real state change"
// gate described in the task card (the cooldown gate lives in dispatch).
func (s *Service) messagesFor(ev domain.Event) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.players[ev.InstanceID]
	msgs := alertsFor(ev, &snap)
	s.players[ev.InstanceID] = snap

	if ev.Name == domain.EventInstanceStatus {
		if st := instanceStatusFromData(ev.Data); st != nil {
			prev := s.downState[ev.InstanceID]
			s.downState[ev.InstanceID] = st.State
			if st.State == domain.StateFailed && prev == domain.StateFailed {
				msgs = withoutKind(msgs, domain.AlertDown)
			}
		}
	}
	return msgs
}

func withoutKind(msgs []Message, kind string) []Message {
	out := msgs[:0]
	for _, m := range msgs {
		if m.Kind != kind {
			out = append(out, m)
		}
	}
	return out
}

// dispatch sends m to every enabled channel subscribed to m.Kind (and, when
// set, whose Instances includes m.InstanceID), subject to the per-(channel,
// kind, instance) cooldown.
func (s *Service) dispatch(ctx context.Context, m Message) {
	for _, ch := range s.settings().Channels {
		if !ch.Enabled || !containsStr(ch.Events, m.Kind) {
			continue
		}
		if len(ch.Instances) > 0 && m.InstanceID != "" && !containsStr(ch.Instances, m.InstanceID) {
			continue
		}
		if !s.allowSend(ch.ID, m.Kind, m.InstanceID) {
			continue
		}
		s.send(ctx, ch, m)
	}
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// allowSend reports whether (channelID, kind, instanceID) is outside its
// cooldown window, recording the attempt time when it is.
func (s *Service) allowSend(channelID, kind, instanceID string) bool {
	key := channelID + "|" + kind + "|" + instanceID
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.lastSent[key]; ok && now.Sub(last) < cooldown {
		return false
	}
	s.lastSent[key] = now
	return true
}

// send delivers m to ch in its own goroutine (never blocking the bus) and
// records the attempt in the log repo.
func (s *Service) send(ctx context.Context, ch domain.NotifyChannel, m Message) {
	sender := s.senderFor(ch.Type)
	if sender == nil {
		return
	}
	go func() {
		sctx, cancel := context.WithTimeout(ctx, sendTimeout)
		defer cancel()
		err := sender.Send(sctx, ch, m)
		s.recordAttempt(ch.ID, m.Kind, m.InstanceID, err)
		if err != nil {
			s.log.Warn("notify: delivery failed", "channel", ch.ID, "type", ch.Type, "kind", m.Kind, "instance", m.InstanceID, "err", err)
		}
	}()
}

func (s *Service) senderFor(t domain.NotifyChannelType) Sender {
	if s.sender != nil {
		return s.sender
	}
	return senderFor(t)
}

func (s *Service) recordAttempt(channelID, kind, instanceID string, sendErr error) {
	if s.logRepo == nil {
		return
	}
	entry := domain.NotificationLogEntry{At: s.now(), ChannelID: channelID, Kind: kind, InstanceID: instanceID, OK: sendErr == nil}
	if sendErr != nil {
		entry.Error = sendErr.Error()
	}
	lctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if err := s.logRepo.Insert(lctx, entry); err != nil {
		s.log.Warn("notify: record delivery attempt failed", "channel", channelID, "kind", kind, "err", err)
	}
}

// checkDisk samples free space on dataDir and raises AlertDiskLow when it
// drops below the configured (or default) threshold, subject to the same
// per-channel cooldown as every other alert.
func (s *Service) checkDisk(ctx context.Context) {
	free, total, err := metrics.DiskUsage(s.dataDir)
	if err != nil || total <= 0 {
		return
	}
	pct := s.settings().DiskLowPercent
	if pct <= 0 {
		pct = defaultDiskLowPercent
	}
	freePercent := free * 100 / total
	if freePercent >= int64(pct) {
		return
	}
	m := Message{
		Kind:  domain.AlertDiskLow,
		Title: "Disk space low",
		Body:  fmt.Sprintf("%d%% free (threshold %d%%)", freePercent, pct),
		At:    s.now(),
	}
	s.dispatch(ctx, m)
}

// Test sends a fixed test message to one channel, ignoring its Events and
// Enabled gates (an explicit "Send test" action), and logs the attempt.
func (s *Service) Test(ctx context.Context, channelID string) error {
	var target *domain.NotifyChannel
	for _, ch := range s.settings().Channels {
		if ch.ID == channelID {
			c := ch
			target = &c
			break
		}
	}
	if target == nil {
		return domain.NotFound("notification channel")
	}
	sender := s.senderFor(target.Type)
	if sender == nil {
		return domain.Ef(domain.CodeValidationFailed, "unsupported channel type %q", target.Type)
	}

	m := Message{Kind: "test", Title: "Test notification", Body: "Test notification from Valheim Server UI", At: s.now()}
	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	err := sender.Send(sctx, *target, m)
	s.recordAttempt(target.ID, m.Kind, "", err)
	return err
}

// Log returns recent notification delivery attempts, newest first.
func (s *Service) Log(ctx context.Context, limit int, before *time.Time) ([]domain.NotificationLogEntry, error) {
	if s.logRepo == nil {
		return nil, nil
	}
	return s.logRepo.List(ctx, limit, before)
}
