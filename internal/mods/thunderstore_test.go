package mods

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestThunderstore_RefreshWritesCache(t *testing.T) {
	ts, _ := newMiniThunderstore(t)

	if ts.IndexUpdatedAt().IsZero() {
		t.Fatal("expected IndexUpdatedAt to be set after Refresh")
	}
	if _, err := os.Stat(ts.indexPath()); err != nil {
		t.Errorf("expected cached index.json: %v", err)
	}
	if _, err := os.Stat(ts.tsPath()); err != nil {
		t.Errorf("expected cached index.ts: %v", err)
	}

	// A fresh client pointed at the same cache dir should load it without
	// hitting the network.
	reloaded := newThunderstoreClient(nil, filepath.Dir(filepath.Dir(ts.cacheDir)), func() time.Duration { return time.Hour }, "test-agent", nil)
	if _, ok := reloaded.LatestVersion("Alice", "CoreLib"); !ok {
		t.Error("expected reloaded client to have loaded the cached index")
	}
}

func TestThunderstore_PackageAndLatestVersion(t *testing.T) {
	ts, _ := newMiniThunderstore(t)

	pkg, err := ts.Package(context.Background(), "Alice", "CoreLib")
	if err != nil {
		t.Fatalf("Package: %v", err)
	}
	if pkg.FullName != "Alice-CoreLib" || len(pkg.Versions) != 1 || pkg.Versions[0].Version != "1.0.0" {
		t.Errorf("unexpected package: %+v", pkg)
	}
	if pkg.WebsiteURL != "https://example.com/corelib" {
		t.Errorf("expected website_url from latest version, got %q", pkg.WebsiteURL)
	}

	if _, err := ts.Package(context.Background(), "nobody", "nothing"); err == nil {
		t.Fatal("expected an error for an unknown package")
	} else if de := domain.AsError(err); de.Code != domain.CodePackageNotFound {
		t.Errorf("expected package_not_found, got %v", de.Code)
	}

	v, ok := ts.LatestVersion("denikson", "BepInExPack_Valheim")
	if !ok || v != "5.4.2202" {
		t.Errorf("expected bepinex latest version 5.4.2202, got %q ok=%v", v, ok)
	}
}

func TestThunderstore_Categories(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	cats, err := ts.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	want := []string{"Libraries", "Mods", "Tools"}
	if len(cats) != len(want) {
		t.Fatalf("expected %v, got %v", want, cats)
	}
	for i, c := range want {
		if cats[i] != c {
			t.Errorf("expected sorted categories %v, got %v", want, cats)
			break
		}
	}
}

func TestThunderstore_Download_CachesZip(t *testing.T) {
	ts, srv := newMiniThunderstore(t)
	_ = srv

	path, err := ts.Download(context.Background(), "Alice", "CoreLib", "1.0.0")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if err := verifyZip(path); err != nil {
		t.Errorf("downloaded file is not a valid zip: %v", err)
	}

	info1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Break the server; a second Download must reuse the cached file rather
	// than re-fetching.
	srv.zips["Alice/CoreLib/1.0.0"] = nil
	path2, err := ts.Download(context.Background(), "Alice", "CoreLib", "1.0.0")
	if err != nil {
		t.Fatalf("second Download (should hit cache): %v", err)
	}
	if path2 != path {
		t.Errorf("expected same cached path, got %q vs %q", path2, path)
	}
	info2, err := os.Stat(path2)
	if err != nil {
		t.Fatal(err)
	}
	if info1.Size() != info2.Size() {
		t.Errorf("expected identical cached file, sizes differ: %d vs %d", info1.Size(), info2.Size())
	}
}

func TestThunderstore_Download_UnknownVersion(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	if _, err := ts.Download(context.Background(), "Alice", "CoreLib", "9.9.9"); err == nil {
		t.Fatal("expected an error downloading a version the fake server does not serve")
	}
}

func searchIndex() []rawPackage {
	now := time.Now().UTC()
	mk := func(owner, name string, rating int, downloads int64, updated time.Time, deprecated bool, cats []string) rawPackage {
		full := owner + "-" + name
		return rawPackage{
			Name: name, FullName: full, Owner: owner, RatingScore: rating, IsDeprecated: deprecated,
			DateCreated: now, DateUpdated: updated, Categories: cats,
			Versions: []rawVersion{{
				Name: name, FullName: full + "-1.0.0", VersionNumber: "1.0.0",
				Description: "desc for " + name, Downloads: downloads, DateCreated: now,
				DownloadURL: "https://thunderstore.io/package/download/" + owner + "/" + name + "/1.0.0/",
			}},
		}
	}
	return []rawPackage{
		mk("Alice", "Alpha", 10, 100, now, false, []string{"Mods"}),
		mk("Bob", "Bravo", 50, 500, now.Add(-time.Hour), false, []string{"Mods", "Libraries"}),
		mk("Carol", "Charlie", 5, 900, now.Add(-2*time.Hour), false, []string{"Tools"}),
		mk("Dave", "Delta", 90, 10, now.Add(-3*time.Hour), true, []string{"Mods"}), // deprecated
		mk("Eve", "Echo", 20, 20, now.Add(1*time.Hour), false, []string{"Libraries"}),
	}
}

func newSearchThunderstore(t *testing.T) *Thunderstore {
	t.Helper()
	srv := newTestThunderstoreServer(t, searchIndex())
	ts := newThunderstoreClient(srv.client(), t.TempDir(), func() time.Duration { return time.Hour }, "test-agent", nil)
	if err := ts.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return ts
}

func TestThunderstore_Search_ExcludesDeprecatedByDefault(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 4 {
		t.Errorf("expected 4 non-deprecated packages, got %d", res.Total)
	}
	for _, p := range res.Packages {
		if p.FullName == "Dave-Delta" {
			t.Error("deprecated package should be excluded by default")
		}
	}

	res, err = ts.Search(context.Background(), domain.PackageSearch{IncludeDeprecated: true, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 5 {
		t.Errorf("expected 5 packages with include_deprecated, got %d", res.Total)
	}
}

func TestThunderstore_Search_SortRating(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Sort: "rating", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"Bob-Bravo", "Eve-Echo", "Alice-Alpha", "Carol-Charlie"}
	assertOrder(t, res.Packages, want)
}

func TestThunderstore_Search_SortDownloads(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Sort: "downloads", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"Carol-Charlie", "Bob-Bravo", "Alice-Alpha", "Eve-Echo"}
	assertOrder(t, res.Packages, want)
}

func TestThunderstore_Search_SortName(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Sort: "name", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"Alice-Alpha", "Bob-Bravo", "Carol-Charlie", "Eve-Echo"}
	assertOrder(t, res.Packages, want)
}

func TestThunderstore_Search_Category(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Category: "Libraries", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	assertOrder(t, res.Packages, []string{"Bob-Bravo", "Eve-Echo"})
}

func TestThunderstore_Search_QuerySubstring(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Query: "bob", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	assertOrder(t, res.Packages, []string{"Bob-Bravo"})
}

func TestThunderstore_Search_Paging(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{Sort: "name", Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 4 || res.Page != 2 || res.PageSize != 2 {
		t.Fatalf("unexpected paging: %+v", res)
	}
	assertOrder(t, res.Packages, []string{"Carol-Charlie", "Eve-Echo"})
}

func TestThunderstore_Search_PageSizeClamped(t *testing.T) {
	ts := newSearchThunderstore(t)
	res, err := ts.Search(context.Background(), domain.PackageSearch{PageSize: 1000})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.PageSize != 100 {
		t.Errorf("expected page_size clamped to 100, got %d", res.PageSize)
	}
}

func assertOrder(t *testing.T, got []domain.PackageSummary, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d results %v, got %d: %+v", len(want), want, len(got), got)
	}
	for i, w := range want {
		if got[i].FullName != w {
			names := make([]string, len(got))
			for j, p := range got {
				names[j] = p.FullName
			}
			t.Fatalf("expected order %v, got %v", want, names)
		}
	}
}
