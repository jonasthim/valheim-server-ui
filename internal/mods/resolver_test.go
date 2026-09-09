package mods

import (
	"context"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.10.0", "1.9.0", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); sign(got) != sign(c.want) {
			t.Errorf("compareVersions(%q,%q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestParseDependency(t *testing.T) {
	owner, name, version, err := parseDependency("denikson-BepInExPack_Valheim-5.4.2202")
	if err != nil {
		t.Fatalf("parseDependency: %v", err)
	}
	if owner != "denikson" || name != "BepInExPack_Valheim" || version != "5.4.2202" {
		t.Errorf("got %q %q %q", owner, name, version)
	}

	owner, name, version, err = parseDependency("Alice-CoreLib-1.0.0")
	if err != nil {
		t.Fatalf("parseDependency: %v", err)
	}
	if owner != "Alice" || name != "CoreLib" || version != "1.0.0" {
		t.Errorf("got %q %q %q", owner, name, version)
	}

	if _, _, _, err := parseDependency("not-a-valid-dependency"); err == nil {
		t.Error("expected an error for a dependency string with no version suffix")
	}
}

func TestResolvePlan_TwoDependencies(t *testing.T) {
	ts, _ := newMiniThunderstore(t)

	plan, needsBepInEx, err := resolvePlan(context.Background(), ts, nil, "Alice", "Awesome", "")
	if err != nil {
		t.Fatalf("resolvePlan: %v", err)
	}
	if !needsBepInEx {
		t.Error("expected the plan to require BepInEx (a transitive dependency)")
	}

	byName := map[string]PlanStep{}
	for _, st := range plan {
		byName[st.FullName()] = st
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 steps (Awesome + CoreLib), got %d: %+v", len(plan), plan)
	}
	if _, ok := byName["Alice-Awesome"]; !ok {
		t.Error("expected Awesome in the plan")
	}
	core, ok := byName["Alice-CoreLib"]
	if !ok {
		t.Fatal("expected CoreLib in the plan (Awesome's dependency)")
	}
	if core.Version != "1.0.0" {
		t.Errorf("expected CoreLib@1.0.0, got %q", core.Version)
	}
	if core.AlreadyInstalled || core.Upgrade {
		t.Errorf("expected a fresh install for CoreLib, got %+v", core)
	}
}

func TestResolvePlan_SkipsAlreadyInstalled(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	installed := map[string]domain.Mod{
		"alice-corelib": {Owner: "Alice", Name: "CoreLib", Version: "1.0.0"},
	}
	plan, _, err := resolvePlan(context.Background(), ts, installed, "Alice", "Awesome", "")
	if err != nil {
		t.Fatalf("resolvePlan: %v", err)
	}
	for _, st := range plan {
		if st.FullName() == "Alice-CoreLib" && !st.AlreadyInstalled {
			t.Errorf("expected CoreLib at the required version to be marked already installed: %+v", st)
		}
	}
}

func TestResolvePlan_UpgradesLowerVersion(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	installed := map[string]domain.Mod{
		"alice-corelib": {Owner: "Alice", Name: "CoreLib", Version: "0.5.0"},
	}
	plan, _, err := resolvePlan(context.Background(), ts, installed, "Alice", "Awesome", "")
	if err != nil {
		t.Fatalf("resolvePlan: %v", err)
	}
	found := false
	for _, st := range plan {
		if st.FullName() == "Alice-CoreLib" {
			found = true
			if !st.Upgrade || st.FromVersion != "0.5.0" || st.Version != "1.0.0" {
				t.Errorf("expected an upgrade from 0.5.0 to 1.0.0, got %+v", st)
			}
		}
	}
	if !found {
		t.Fatal("expected CoreLib in the plan")
	}
}

func TestResolvePlan_UnknownPackage(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	if _, _, err := resolvePlan(context.Background(), ts, nil, "Nobody", "Nothing", ""); err == nil {
		t.Fatal("expected an error resolving an unknown package")
	} else if de := domain.AsError(err); de.Code != domain.CodePackageNotFound {
		t.Errorf("expected package_not_found, got %v", de.Code)
	}
}
