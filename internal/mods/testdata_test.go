package mods

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// rewriteTransport redirects every request to base's scheme+host, keeping the
// original path/query, so production code that hard-codes the real
// thunderstore.io URLs can be pointed at an httptest.Server in tests.
type rewriteTransport struct {
	base *url.URL
	rt   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.base.Scheme
	req.URL.Host = t.base.Host
	req.Host = t.base.Host
	return t.rt.RoundTrip(req)
}

// zipEntry is one file to write into a test zip.
type zipEntry struct {
	Name string
	Data []byte
}

func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.Name)
		if err != nil {
			t.Fatalf("zip create %s: %v", e.Name, err)
		}
		if _, err := w.Write(e.Data); err != nil {
			t.Fatalf("zip write %s: %v", e.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// bepinexPackZip builds a minimal but structurally faithful BepInEx pack zip:
// a single top-level "BepInExPack_Valheim/" wrapper folder whose contents are
// meant to be copied verbatim into the server dir root (ARCHITECTURE.md §12).
func bepinexPackZip(t *testing.T) []byte {
	t.Helper()
	// Real packs carry Thunderstore metadata at the archive root next to the
	// payload folder; those files must not land in the server dir.
	return buildZip(t, []zipEntry{
		{"manifest.json", []byte(`{"name":"BepInExPack_Valheim","version_number":"5.4.2202","dependencies":[]}`)},
		{"README.md", []byte("# pack")},
		{"icon.png", []byte("png")},
		{"CHANGELOG.md", []byte("changes")},
		{"BepInExPack_Valheim/BepInEx/core/BepInEx.Preloader.dll", []byte("preloader")},
		{"BepInExPack_Valheim/BepInEx/core/BepInEx.dll", []byte("core")},
		{"BepInExPack_Valheim/doorstop_libs/libdoorstop_x64.so", []byte("doorstop")},
		{"BepInExPack_Valheim/start_server_bepinex.sh", []byte("#!/bin/sh\n")},
		{"BepInExPack_Valheim/winhttp.dll", []byte("winhttp")},
		{"BepInExPack_Valheim/doorstop_config.ini", []byte("[General]\nenabled=true\n")},
		{"BepInExPack_Valheim/changelog.txt", []byte("v5.4.2202\n")},
	})
}

// coreLibZip builds a flat (no wrapper folder) Thunderstore package zip with
// a manifest, icon, README, a plugin dll and a default config file, per the
// WORKPLAN.md WP-08 test plan ("a mod with plugins/ + config/ + README").
func coreLibZip(t *testing.T) []byte {
	t.Helper()
	manifest := `{"name":"CoreLib","version_number":"1.0.0","website_url":"https://example.com/corelib","description":"core library","dependencies":[]}`
	return buildZip(t, []zipEntry{
		{"manifest.json", []byte(manifest)},
		{"icon.png", []byte("PNG")},
		{"README.md", []byte("# CoreLib\n")},
		{"plugins/CoreLib.dll", []byte("corelib-dll-v1")},
		{"config/corelib.cfg", []byte("[General]\nEnabled = true\n")},
	})
}

// coreLibZipV2 is an upgraded CoreLib whose plugin dll content differs (so
// tests can tell old vs new bytes apart) and whose config entry, if
// extracted, must never overwrite an existing one.
func coreLibZipV2(t *testing.T) []byte {
	t.Helper()
	manifest := `{"name":"CoreLib","version_number":"1.1.0","website_url":"https://example.com/corelib","description":"core library","dependencies":[]}`
	return buildZip(t, []zipEntry{
		{"manifest.json", []byte(manifest)},
		{"plugins/CoreLib.dll", []byte("corelib-dll-v2")},
		{"config/corelib.cfg", []byte("[General]\nEnabled = true\n")},
	})
}

// awesomeZip depends (per the index, not the zip) on CoreLib and BepInEx.
func awesomeZip(t *testing.T) []byte {
	t.Helper()
	manifest := `{"name":"Awesome","version_number":"2.0.0","website_url":"https://example.com/awesome","description":"an awesome mod","dependencies":["Alice-CoreLib-1.0.0","denikson-BepInExPack_Valheim-5.4.2202"]}`
	return buildZip(t, []zipEntry{
		{"manifest.json", []byte(manifest)},
		{"plugins/Awesome.dll", []byte("awesome-dll")},
	})
}

// miniIndex is the hand-made mini Thunderstore index used across mods
// package tests: the BepInEx pack plus a mod (Awesome) that depends on
// another mod (CoreLib) and on BepInEx itself (WORKPLAN.md WP-08).
func miniIndex() []rawPackage {
	now := time.Now().UTC()
	return []rawPackage{
		{
			Name: "BepInExPack_Valheim", FullName: "denikson-BepInExPack_Valheim", Owner: "denikson",
			RatingScore: 100, DateCreated: now, DateUpdated: now,
			Categories: []string{"Tools"},
			Versions: []rawVersion{{
				Name: "BepInExPack_Valheim", FullName: "denikson-BepInExPack_Valheim-5.4.2202",
				VersionNumber: "5.4.2202", Description: "BepInEx pack for Valheim", Downloads: 100000,
				DateCreated: now, DownloadURL: "https://thunderstore.io/package/download/denikson/BepInExPack_Valheim/5.4.2202/",
			}},
		},
		{
			Name: "CoreLib", FullName: "Alice-CoreLib", Owner: "Alice",
			RatingScore: 50, DateCreated: now, DateUpdated: now.Add(-time.Hour),
			Categories: []string{"Libraries"},
			Versions: []rawVersion{{
				Name: "CoreLib", FullName: "Alice-CoreLib-1.0.0", VersionNumber: "1.0.0",
				Description: "core library", Downloads: 200, DateCreated: now,
				DownloadURL: "https://thunderstore.io/package/download/Alice/CoreLib/1.0.0/",
				WebsiteURL:  "https://example.com/corelib",
			}},
		},
		{
			Name: "Awesome", FullName: "Alice-Awesome", Owner: "Alice",
			RatingScore: 80, DateCreated: now, DateUpdated: now,
			Categories: []string{"Mods"},
			Versions: []rawVersion{{
				Name: "Awesome", FullName: "Alice-Awesome-2.0.0", VersionNumber: "2.0.0",
				Description: "an awesome mod", Downloads: 500, DateCreated: now,
				DownloadURL:  "https://thunderstore.io/package/download/Alice/Awesome/2.0.0/",
				Dependencies: []string{"Alice-CoreLib-1.0.0", "denikson-BepInExPack_Valheim-5.4.2202"},
			}},
		},
	}
}

// testThunderstoreServer serves a raw index (mutable via setIndex) and
// package zips keyed "owner/name/version" over HTTP.
type testThunderstoreServer struct {
	*httptest.Server
	index []rawPackage
	zips  map[string][]byte
}

func newTestThunderstoreServer(t *testing.T, index []rawPackage) *testThunderstoreServer {
	t.Helper()
	ts := &testThunderstoreServer{index: index, zips: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/c/valheim/api/v1/package/", func(w http.ResponseWriter, r *http.Request) {
		data, err := json.Marshal(ts.index)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/package/download/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 5 {
			http.NotFound(w, r)
			return
		}
		key := parts[2] + "/" + parts[3] + "/" + parts[4]
		data, ok := ts.zips[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	})
	ts.Server = httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func (ts *testThunderstoreServer) setZip(owner, name, version string, data []byte) {
	ts.zips[owner+"/"+name+"/"+version] = data
}

func (ts *testThunderstoreServer) client() *http.Client {
	u, err := url.Parse(ts.URL)
	if err != nil {
		panic(err)
	}
	return &http.Client{Transport: &rewriteTransport{base: u, rt: http.DefaultTransport}}
}

// newMiniThunderstore builds a *Thunderstore backed by a testThunderstoreServer
// preloaded with miniIndex() and the three corresponding zips, refreshed once
// so the in-memory index is populated.
func newMiniThunderstore(t *testing.T) (*Thunderstore, *testThunderstoreServer) {
	t.Helper()
	srv := newTestThunderstoreServer(t, miniIndex())
	srv.setZip("denikson", "BepInExPack_Valheim", "5.4.2202", bepinexPackZip(t))
	srv.setZip("Alice", "CoreLib", "1.0.0", coreLibZip(t))
	srv.setZip("Alice", "Awesome", "2.0.0", awesomeZip(t))

	cacheDir := t.TempDir()
	ts := NewThunderstore(srv.client(), cacheDir, func() time.Duration { return time.Hour }, "test-agent", nil)
	if err := ts.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return ts, srv
}
