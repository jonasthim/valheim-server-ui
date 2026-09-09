package players

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Service implements api.PlayerService (structurally; this package does not
// import internal/api to avoid a cycle — wiring happens in
// cmd/valheim-ui/wire_players.go).
type Service struct {
	mgr    *Manager
	store  PlayerStore
	paths  func(string) domain.InstancePaths
	exists func(context.Context, string) (bool, error)
}

// NewService builds the players.Service. paths resolves an instance's
// on-disk layout (for the list files); exists checks the instance exists,
// returning domain.NotFound("instance") to callers when it does not.
func NewService(mgr *Manager, store PlayerStore, paths func(string) domain.InstancePaths, exists func(context.Context, string) (bool, error)) *Service {
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
