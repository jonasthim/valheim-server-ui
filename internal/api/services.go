package api

import (
	"context"
	"io"
	"net/http"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Service interfaces consumed by handlers. Each is implemented by the package
// named in WORKPLAN.md. Handlers must depend on these, never on concrete types,
// so waves can be developed and tested independently with fakes.

// WP-02
type InstanceService interface {
	List(ctx context.Context) ([]domain.Instance, error)
	Get(ctx context.Context, id string) (*domain.Instance, error)
	Create(ctx context.Context, id, name string, cfg domain.InstanceConfig, autostart bool) (*domain.Instance, error)
	Update(ctx context.Context, id string, name *string, cfg *domain.InstanceConfig, autostart *bool) (*domain.Instance, error)
	Delete(ctx context.Context, id string, deleteFiles bool) error
	Start(ctx context.Context, id string) (*domain.InstanceStatus, error)
	Stop(ctx context.Context, id string) (*domain.InstanceStatus, error)
	Restart(ctx context.Context, id string) (*domain.InstanceStatus, error)
	Status(ctx context.Context, id string) (*domain.InstanceStatus, error)
	TailLog(ctx context.Context, id string, lines int) ([]string, error)
	OpenLog(ctx context.Context, id string) (io.ReadCloser, error)
}

// WP-04 (+ WP-05 for the instance-scoped job kinds)
type JobService interface {
	List(ctx context.Context, instanceID string, status domain.JobStatus, limit int) ([]domain.Job, error)
	Get(ctx context.Context, id string) (*domain.Job, error)
	Log(ctx context.Context, id string) ([]string, error)
	Cancel(ctx context.Context, id string) (*domain.Job, error)
}

// WP-04/05
type SteamService interface {
	EnqueueInstall(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error)
	EnqueueUpdate(ctx context.Context, instanceID, requestedBy string, stopIfRunning bool) (*domain.Job, error)
	CheckUpdate(ctx context.Context, instanceID string) (*domain.UpdateInfo, error)
	SteamCMDInstalled() bool
	LatestBuildID() (string, *domain.UpdateInfo)
}

// WP-01
type UserService interface {
	List(ctx context.Context) ([]domain.User, error)
	Get(ctx context.Context, id int64) (*domain.User, error)
	Create(ctx context.Context, username, password, displayName, email string, role domain.Role) (*domain.User, error)
	Update(ctx context.Context, id int64, displayName, email *string, role *domain.Role, disabled *bool) (*domain.User, error)
	Delete(ctx context.Context, id int64) error
	SetPassword(ctx context.Context, id int64, newPassword string) error
	ChangePassword(ctx context.Context, id int64, current, newPassword string) error
	Count(ctx context.Context) (int, error)
}

// WP-01
type SettingsService interface {
	Get(ctx context.Context) (domain.Settings, error)
	// Put validates and stores; empty OIDC client secret keeps the stored one.
	Put(ctx context.Context, s domain.Settings) (domain.Settings, error)
	// Redacted returns settings with secrets blanked for API responses.
	Redacted(s domain.Settings) domain.Settings
	// TestOIDC discovers issuerURL and reports whether it looks valid,
	// used by POST /settings/oidc/test.
	TestOIDC(ctx context.Context, issuerURL string) (ok bool, issuer, authEndpoint, errMsg string)
}

// WP-03
type PlayerService interface {
	Players(ctx context.Context, instanceID string) (*domain.PlayersResponse, error)
	GetList(ctx context.Context, instanceID string, kind domain.ListKind) (*domain.PlayerList, error)
	PutList(ctx context.Context, instanceID string, list domain.PlayerList) (*domain.PlayerList, error)
}

// WP-06
type BackupService interface {
	List(ctx context.Context, instanceID string) ([]domain.Backup, error)
	EnqueueBackup(ctx context.Context, instanceID string, kind domain.BackupKind, note, requestedBy string) (*domain.Job, error)
	EnqueueRestore(ctx context.Context, instanceID string, backupID int64, stopIfRunning bool, requestedBy string) (*domain.Job, error)
	Delete(ctx context.Context, instanceID string, backupID int64) error
	Open(ctx context.Context, instanceID string, backupID int64) (*domain.Backup, io.ReadCloser, error)
	Upload(ctx context.Context, instanceID, filename string, r io.Reader) (*domain.Backup, error)

	ListWorlds(ctx context.Context, instanceID string) ([]domain.World, error)
	EnqueueWorldImport(ctx context.Context, instanceID string, files map[string]io.Reader, overwrite bool, requestedBy string) (*domain.Job, error)
	DeleteWorld(ctx context.Context, instanceID, world string) error
	// ExportWorld writes a zip of <world>.db/.fwl to w.
	ExportWorld(ctx context.Context, instanceID, world string, w io.Writer) error
}

// WP-07
type ScheduleService interface {
	List(ctx context.Context, instanceID string) ([]domain.Schedule, error)
	Create(ctx context.Context, instanceID string, in domain.ScheduleInput) (*domain.Schedule, error)
	Update(ctx context.Context, instanceID string, id int64, in domain.ScheduleInput) (*domain.Schedule, error)
	Delete(ctx context.Context, instanceID string, id int64) error
	RunNow(ctx context.Context, instanceID string, id int64, requestedBy string) (*domain.Job, error)
}

// WP-08
type ModService interface {
	Overview(ctx context.Context, instanceID string) (*domain.ModsOverview, error)
	EnqueueInstall(ctx context.Context, instanceID, owner, name, version, requestedBy string) (*domain.Job, error)
	EnqueueUpload(ctx context.Context, instanceID, filename string, r io.Reader, requestedBy string) (*domain.Job, error)
	EnqueueBepInExInstall(ctx context.Context, instanceID string, stopIfRunning bool, requestedBy string) (*domain.Job, error)
	SetBepInExEnabled(ctx context.Context, instanceID string, enabled bool) (*domain.ModsOverview, error)
	SetEnabled(ctx context.Context, instanceID string, modID int64, enabled bool) (*domain.Mod, error)
	EnqueueUninstall(ctx context.Context, instanceID string, modID int64, requestedBy string) (*domain.Job, error)
	EnqueueUpdate(ctx context.Context, instanceID string, modID int64, version, requestedBy string) (*domain.Job, error)
	ListConfigs(ctx context.Context, instanceID string) ([]domain.ConfigFileInfo, error)
	GetConfig(ctx context.Context, instanceID, file string) (*domain.ConfigFile, error)
	UpdateConfig(ctx context.Context, instanceID, file string, upd domain.ConfigFileUpdate) (*domain.ConfigFile, error)
}

// WP-08
type ThunderstoreService interface {
	Search(ctx context.Context, q domain.PackageSearch) (*domain.PackageSearchResult, error)
	Package(ctx context.Context, owner, name string) (*domain.Package, error)
	Categories(ctx context.Context) ([]string, error)
	EnqueueRefresh(ctx context.Context, requestedBy string) (*domain.Job, error)
}

// MultipartFile is a helper shape for upload handlers.
type MultipartFile struct {
	Name string
	Body io.Reader
}

// RequestedBy returns the username for job attribution.
func RequestedBy(r *http.Request) string {
	if u := UserFrom(r.Context()); u != nil {
		return u.Username
	}
	return ""
}
