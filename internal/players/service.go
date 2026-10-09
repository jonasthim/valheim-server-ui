package players

import (
	"context"
	"errors"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// serviceStore is the persistence surface the players Service needs: the
// shared PlayerStore (List, for PlayersResponse.Known) plus SetNote, which
// only the API layer calls — the tracker never does — and so is not part of
// PlayerStore itself.
type serviceStore interface {
	PlayerStore
	SetNote(ctx context.Context, instanceID, platformID, note string) error
	ListComments(ctx context.Context, instanceID string, kind domain.ListKind) (map[string]string, error)
	ReplaceListComments(ctx context.Context, instanceID string, kind domain.ListKind, entries []domain.PlayerListEntry) error
}

// Service implements api.PlayerService (structurally; this package does not
// import internal/api to avoid a cycle — wiring happens in
// cmd/valheim-ui/wire_players.go).
type Service struct {
	mgr    *Manager
	store  serviceStore
	paths  func(string) domain.InstancePaths
	exists func(context.Context, string) (bool, error)
}

// NewService builds the players.Service. paths resolves an instance's
// on-disk layout (for the list files); exists checks the instance exists,
// returning domain.NotFound("instance") to callers when it does not.
func NewService(mgr *Manager, store serviceStore, paths func(string) domain.InstancePaths, exists func(context.Context, string) (bool, error)) *Service {
	return &Service{mgr: mgr, store: store, paths: paths, exists: exists}
}

func (s *Service) checkExists(ctx context.Context, instanceID string) error {
	if s.exists == nil {
		return nil
	}
	ok, err := s.exists(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("check instance %s: %w", instanceID, err)
	}
	if !ok {
		return domain.NotFound("instance")
	}
	return nil
}

// Players implements api.PlayerService.
func (s *Service) Players(ctx context.Context, instanceID string) (*domain.PlayersResponse, error) {
	if err := s.checkExists(ctx, instanceID); err != nil {
		return nil, err
	}

	online := []domain.OnlinePlayer{}
	countSource := "none"
	count := 0
	if s.mgr != nil {
		got, _, _, a2s, a2sAge, tracked := s.mgr.Snapshot(instanceID)
		if got != nil {
			online = got
		}
		if tracked {
			if a2s != nil && a2sAge <= a2sFreshWindow {
				countSource = "a2s"
				count = a2s.Players
			} else {
				countSource = "log"
				count = len(online)
			}
		}
	}

	var known []domain.KnownPlayer
	if s.store != nil {
		var err error
		known, err = s.store.List(ctx, instanceID, 200)
		if err != nil {
			return nil, fmt.Errorf("list known players: %w", err)
		}
	}
	if known == nil {
		known = []domain.KnownPlayer{}
	}

	return &domain.PlayersResponse{
		Online:      online,
		OnlineCount: count,
		CountSource: countSource,
		Known:       known,
	}, nil
}

// GetList implements api.PlayerService.
func (s *Service) GetList(ctx context.Context, instanceID string, kind domain.ListKind) (*domain.PlayerList, error) {
	if err := s.checkExists(ctx, instanceID); err != nil {
		return nil, err
	}
	if !kind.Valid() {
		return nil, domain.NotFound("list")
	}
	p := s.paths(instanceID)
	file := p.ListFile(kind)
	list, err := ReadList(file, kind)
	if err != nil || s.store == nil {
		return list, err
	}
	comments, err := s.store.ListComments(ctx, instanceID, kind)
	if err != nil {
		return nil, err
	}
	legacyInline := false
	for i := range list.Entries {
		entry := &list.Entries[i]
		if entry.Comment != "" {
			legacyInline = true
			if _, exists := comments[entry.ID]; !exists {
				comments[entry.ID] = entry.Comment
			}
		}
		entry.Comment = comments[entry.ID]
	}
	if legacyInline {
		// Move comments written by older manager versions out of the live
		// Valheim file before answering. Keep its standalone header intact.
		if err := s.store.ReplaceListComments(ctx, instanceID, kind, list.Entries); err != nil {
			return nil, err
		}
		if err := WriteList(file, *list); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// PutList implements api.PlayerService.
func (s *Service) PutList(ctx context.Context, instanceID string, list domain.PlayerList) (*domain.PlayerList, error) {
	if err := s.checkExists(ctx, instanceID); err != nil {
		return nil, err
	}
	if !list.Kind.Valid() {
		return nil, domain.NotFound("list")
	}
	p := s.paths(instanceID)
	file := p.ListFile(list.Kind)
	if err := validateEntries(list.Entries); err != nil {
		return nil, err
	}
	var previous map[string]string
	if s.store != nil {
		var err error
		previous, err = s.store.ListComments(ctx, instanceID, list.Kind)
		if err != nil {
			return nil, err
		}
		if err := s.store.ReplaceListComments(ctx, instanceID, list.Kind, list.Entries); err != nil {
			return nil, err
		}
	}
	if err := WriteList(file, list); err != nil {
		if s.store != nil {
			oldEntries := make([]domain.PlayerListEntry, 0, len(previous))
			for id, comment := range previous {
				oldEntries = append(oldEntries, domain.PlayerListEntry{ID: id, Comment: comment})
			}
			if rollbackErr := s.store.ReplaceListComments(ctx, instanceID, list.Kind, oldEntries); rollbackErr != nil {
				return nil, errors.Join(err, fmt.Errorf("restore list comments: %w", rollbackErr))
			}
		}
		return nil, err
	}
	return s.GetList(ctx, instanceID, list.Kind)
}

// SetNote implements api.PlayerService: stores (or clears, when note is
// empty) a short operator note against a known player. platformID must have
// the same shape accepted by the admin/banned/permitted list files; note is
// capped at 500 characters.
func (s *Service) SetNote(ctx context.Context, instanceID, platformID, note string) error {
	if err := s.checkExists(ctx, instanceID); err != nil {
		return err
	}
	var fields []domain.FieldError
	if !idPattern.MatchString(platformID) {
		fields = append(fields, domain.FieldError{
			Field:   "platform_id",
			Message: "must match ^[A-Za-z0-9_]{1,64}$",
		})
	}
	if len(note) > 500 {
		fields = append(fields, domain.FieldError{
			Field:   "note",
			Message: "must be at most 500 characters",
		})
	}
	if err := domain.Validation(fields); err != nil {
		return err
	}
	return s.store.SetNote(ctx, instanceID, platformID, note)
}
