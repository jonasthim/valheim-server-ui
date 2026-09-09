package selfupdate

import (
	"context"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestClient_LatestAndByTag(t *testing.T) {
	tarball, sums := buildReleaseAssets(t, "v1.2.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.2.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.1.0", WithBaseURL(srv.URL))

	rel, err := client.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel == nil || rel.Tag != "v1.2.0" {
		t.Fatalf("expected release v1.2.0, got %+v", rel)
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(rel.Assets))
	}

	byTag, err := client.ByTag(context.Background(), "v1.2.0")
	if err != nil {
		t.Fatalf("ByTag: %v", err)
	}
	if byTag == nil || byTag.Tag != "v1.2.0" {
		t.Fatalf("expected release v1.2.0, got %+v", byTag)
	}

	missing, err := client.ByTag(context.Background(), "v9.9.9")
	if err != nil {
		t.Fatalf("ByTag(missing): unexpected error %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil release for an unknown tag, got %+v", missing)
	}
}

func TestClient_NoReleasesYet(t *testing.T) {
	srv := newNotFoundServer(t)
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))

	rel, err := client.Latest(context.Background())
	if err != nil {
		t.Fatalf("expected 404 to be reported as no error, got %v", err)
	}
	if rel != nil {
		t.Fatalf("expected nil release, got %+v", rel)
	}
}

func TestClient_RateLimited(t *testing.T) {
	srv := newForbiddenServer(t, "API rate limit exceeded")
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))

	_, err := client.Latest(context.Background())
	if err == nil {
		t.Fatal("expected an error for a 403 response")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeUpstreamError {
		t.Fatalf("expected upstream_error, got %s", de.Code)
	}
	if !strings.Contains(de.Message, "rate limit") {
		t.Fatalf("expected rate limit message to be surfaced, got %q", de.Message)
	}
}
