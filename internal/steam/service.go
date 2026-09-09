package steam

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Service satisfies api.SteamService structurally (verified where it is
// wired to api.Deps.Steam in cmd/valheim-ui). It deliberately does not
// import internal/api itself: internal/api's own internal test files import
// internal/instance (WP-02, for a real Service in handler tests), and
// internal/instance (WP-05) depends on internal/steam, so this package
// importing internal/api back would be an import cycle for those test files.
//
// Service adapts a Client (+ optional UpdateChecker) to api.SteamService.
// EnqueueInstall, EnqueueUpdate and CheckUpdate are instance-scoped
// operations owned by WP-05 (they need the instance service to stop/start
// the server and to persist installed_buildid); until that lands they return
// a clearly-labelled "not wired" error so the interface is satisfiable and
// callers get a sensible 500 rather than a nil-pointer panic.
type Service struct {
	client  *Client
	checker *UpdateChecker // optional; nil is fine (LatestBuildID falls back to client.Latest())
}

// NewService builds the api.SteamService adapter. checker may be nil.
func NewService(client *Client, checker *UpdateChecker) *Service {
	return &Service{client: client, checker: checker}
}

// SteamCMDInstalled reports whether steamcmd is present and executable.
func (s *Service) SteamCMDInstalled() bool {
	return s.client != nil && s.client.Installed()
}

// LatestBuildID returns the last known public build id and, when available,
// an UpdateInfo carrying when it was checked (for GET /system's
// buildid_checked_at).
func (s *Service) LatestBuildID() (string, *domain.UpdateInfo) {
	if s.client == nil {
		return "", nil
	}
	build, checkedAt := s.client.Latest()
	if build == "" {
		return "", nil
	}
	return build, &domain.UpdateInfo{LatestBuildID: build, CheckedAt: &checkedAt}
}

// notWired is returned by the instance-scoped methods below until WP-05
// wires them against the instance service.
func notWired() error { return domain.E(domain.CodeInternal, "not wired (WP-05)") }

// EnqueueInstall is implemented by WP-05.
func (s *Service) EnqueueInstall(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error) {
	return nil, notWired()
}

// EnqueueUpdate is implemented by WP-05.
func (s *Service) EnqueueUpdate(ctx context.Context, instanceID, requestedBy string, stopIfRunning bool) (*domain.Job, error) {
	return nil, notWired()
}

// CheckUpdate is implemented by WP-05.
func (s *Service) CheckUpdate(ctx context.Context, instanceID string) (*domain.UpdateInfo, error) {
	if s.checker != nil {
		if info := s.checker.Info(instanceID); info != nil {
			return info, nil
		}
	}
	return nil, notWired()
}
