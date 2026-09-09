package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestThunderstoreAPI_SearchCategoriesPackage(t *testing.T) {
	a := newModsTestAPI(t)

	rec := a.do(t, http.MethodGet, "/api/v1/thunderstore/packages?q=core", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result domain.PackageSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Packages) != 1 || result.Packages[0].FullName != "Alice-CoreLib" {
		t.Fatalf("unexpected search result: %+v", result)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/thunderstore/categories", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("categories: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var catResp struct {
		Categories []string `json:"categories"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &catResp)
	if len(catResp.Categories) != 1 || catResp.Categories[0] != "Libraries" {
		t.Fatalf("unexpected categories: %v", catResp.Categories)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/thunderstore/packages/Alice/CoreLib", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get package: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var pkg domain.Package
	_ = json.Unmarshal(rec.Body.Bytes(), &pkg)
	if pkg.FullName != "Alice-CoreLib" || len(pkg.Versions) != 1 {
		t.Fatalf("unexpected package: %+v", pkg)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/thunderstore/packages/Nobody/Nothing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown package: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestThunderstoreAPI_Refresh(t *testing.T) {
	a := newModsTestAPI(t)

	rec := a.do(t, http.MethodPost, "/api/v1/thunderstore/refresh", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobResp struct {
		Job domain.Job `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &jobResp); err != nil {
		t.Fatal(err)
	}
	a.waitJob(t, jobResp.Job.ID)
}

func TestThunderstoreAPI_SearchPageSizeValidation(t *testing.T) {
	a := newModsTestAPI(t)

	rec := a.do(t, http.MethodGet, "/api/v1/thunderstore/packages?page=0", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for page=0, got %d: %s", rec.Code, rec.Body.String())
	}
}
