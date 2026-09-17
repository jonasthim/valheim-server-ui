package notify

import (
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// playersSnapshot is the set of currently-online player names for one
// instance, used to diff instance.players events into join/leave alerts. The
// caller (Service) keeps one per instance and passes the right one in.
type playersSnapshot map[string]bool

// alertableJobTypes are the job kinds whose failure raises AlertJobFailed.
var alertableJobTypes = map[domain.JobType]bool{
	domain.JobBackup:           true,
	domain.JobBackupUpload:     true,
	domain.JobRestore:          true,
	domain.JobUpdate:           true,
	domain.JobScheduledRestart: true,
	domain.JobRestart:          true,
	domain.JobSelfUpgrade:      true,
}

// alertsFor maps one bus event to zero or more alert messages. prev is the
// previous online-players snapshot for the event's instance; it is read and,
// for instance.players events, updated in place to the new online set. It is
// ignored for every other event (callers may pass a pointer to a nil map).
func alertsFor(ev domain.Event, prev *playersSnapshot) []Message {
	switch ev.Name {
	case domain.EventInstanceCrashed:
		return crashedAlert(ev)
	case domain.EventInstanceStatus:
		return downAlert(ev)
	case domain.EventJobUpdated:
		return jobFailedAlert(ev)
	case domain.EventUpdateAvailable:
		return gameUpdateAlert(ev)
	case domain.EventAppUpdateAvailable:
		return appUpdateAlert(ev)
	case domain.EventInstancePlayers:
		return playerAlerts(ev, prev)
	case domain.EventAgentChat:
		return chatAlert(ev)
	default:
		return nil
	}
}

func crashedAlert(ev domain.Event) []Message {
	e, ok := instanceEventFromData(ev.Data)
	if !ok {
		return nil
	}
	return []Message{{
		Kind: domain.AlertCrashed, Title: fmt.Sprintf("%s crashed", ev.InstanceID),
		Body: e.Detail, InstanceID: ev.InstanceID, At: ev.TS,
	}}
}

func downAlert(ev domain.Event) []Message {
	st := instanceStatusFromData(ev.Data)
	if st == nil || st.State != domain.StateFailed {
		return nil
	}
	return []Message{{
		Kind: domain.AlertDown, Title: fmt.Sprintf("%s is down", ev.InstanceID),
		Body: st.Detail, InstanceID: ev.InstanceID, At: ev.TS,
	}}
}

func jobFailedAlert(ev domain.Event) []Message {
	j, ok := ev.Data.(domain.Job)
	if !ok || j.Status != domain.JobFailed || !alertableJobTypes[j.Type] {
		return nil
	}
	return []Message{{
		Kind:       domain.AlertJobFailed,
		Title:      fmt.Sprintf("%s failed on %s", j.Type, j.InstanceID),
		Body:       j.Error,
		InstanceID: j.InstanceID,
		At:         ev.TS,
	}}
}

func gameUpdateAlert(ev domain.Event) []Message {
	info, ok := ev.Data.(domain.UpdateInfo)
	if !ok || !info.UpdateAvailable {
		return nil
	}
	return []Message{{
		Kind:       domain.AlertGameUpdate,
		Title:      fmt.Sprintf("Game update available for %s", ev.InstanceID),
		Body:       fmt.Sprintf("build %s available (installed %s)", info.LatestBuildID, info.InstalledBuildID),
		InstanceID: ev.InstanceID,
		At:         ev.TS,
	}}
}

func appUpdateAlert(ev domain.Event) []Message {
	info, ok := ev.Data.(domain.AppUpdateInfo)
	if !ok || !info.UpdateAvailable {
		return nil
	}
	return []Message{{
		Kind:  domain.AlertAppUpdate,
		Title: "Manager update available",
		Body:  fmt.Sprintf("%s -> %s", info.CurrentVersion, info.LatestVersion),
		At:    ev.TS,
	}}
}

// chatAlert reports one in-game chat line (F-2.3).
func chatAlert(ev domain.Event) []Message {
	e, ok := ev.Data.(domain.ChatLogEntry)
	if !ok {
		return nil
	}
	return []Message{{
		Kind: domain.AlertChat, Title: fmt.Sprintf("%s in %s", e.Sender, ev.InstanceID),
		Body: e.Text, InstanceID: ev.InstanceID, At: ev.TS,
	}}
}

// playerAlerts diffs the event's online set against *prev, returning one
// AlertPlayerJoin/AlertPlayerLeave Message per name that changed state, and
// updates *prev to the new online set.
func playerAlerts(ev domain.Event, prev *playersSnapshot) []Message {
	names, ok := onlineNamesFromData(ev.Data)
	if !ok {
		return nil
	}
	cur := playersSnapshot{}
	for _, name := range names {
		if name != "" {
			cur[name] = true
		}
	}
	old := *prev

	var msgs []Message
	for name := range cur {
		if !old[name] {
			msgs = append(msgs, Message{
				Kind: domain.AlertPlayerJoin, Title: fmt.Sprintf("%s joined %s", name, ev.InstanceID),
				Body: name, InstanceID: ev.InstanceID, At: ev.TS,
			})
		}
	}
	for name := range old {
		if !cur[name] {
			msgs = append(msgs, Message{
				Kind: domain.AlertPlayerLeave, Title: fmt.Sprintf("%s left %s", name, ev.InstanceID),
				Body: name, InstanceID: ev.InstanceID, At: ev.TS,
			})
		}
	}
	*prev = cur
	return msgs
}

// onlineNamesFromData extracts the player names from an instance.players
// event payload (see internal/players/tracker.go publish:
// map[string]any{"instance_id":..., "online": []domain.OnlinePlayer}).
func onlineNamesFromData(data any) ([]string, bool) {
	m, ok := data.(map[string]any)
	if !ok {
		return nil, false
	}
	list, ok := m["online"].([]domain.OnlinePlayer)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, len(list))
	for _, p := range list {
		names = append(names, p.Name)
	}
	return names, true
}

// instanceEventFromData accepts either value or pointer form, mirroring
// internal/players/tracker.go's statusFromEvent defensive style.
func instanceEventFromData(data any) (domain.InstanceEvent, bool) {
	switch v := data.(type) {
	case domain.InstanceEvent:
		return v, true
	case *domain.InstanceEvent:
		if v == nil {
			return domain.InstanceEvent{}, false
		}
		return *v, true
	default:
		return domain.InstanceEvent{}, false
	}
}

// instanceStatusFromData accepts either value or pointer form, mirroring
// internal/players/tracker.go's statusFromEvent.
func instanceStatusFromData(data any) *domain.InstanceStatus {
	switch v := data.(type) {
	case domain.InstanceStatus:
		return &v
	case *domain.InstanceStatus:
		return v
	default:
		return nil
	}
}
