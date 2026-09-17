package players

import (
	"context"
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
	return ReadList(p.ListFile(kind), kind)
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
	if err := WriteList(file, list); err != nil {
		return nil, err
	}
	return ReadList(file, list.Kind)
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
