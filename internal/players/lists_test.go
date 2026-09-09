package players

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestReadList_MissingFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	list, err := ReadList(filepath.Join(dir, "adminlist.txt"), domain.ListAdmin)
	if err != nil {
		t.Fatalf("ReadList: %v", err)
	}
	if list.Kind != domain.ListAdmin {
		t.Errorf("Kind = %q, want %q", list.Kind, domain.ListAdmin)
	}
	if len(list.Entries) != 0 {
		t.Errorf("Entries = %v, want empty", list.Entries)
	}
}

func TestReadList_ParsesEntriesAndTrailingComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adminlist.txt")
	content := "// managed by valheim-ui\n" +
		"\n" +
		"76561198000000001 // Bjorn, admin\n" +
		"76561198000000002\n" +
		"// note about the next one\n" +
		"76561198000000003\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	list, err := ReadList(path, domain.ListAdmin)
	if err != nil {
		t.Fatalf("ReadList: %v", err)
	}
	want := []domain.PlayerListEntry{
		{ID: "76561198000000001", Comment: "Bjorn, admin"},
		{ID: "76561198000000002"},
		{ID: "76561198000000003"},
	}
	if len(list.Entries) != len(want) {
		t.Fatalf("Entries = %#v, want %#v", list.Entries, want)
	}
	for i := range want {
		if list.Entries[i] != want[i] {
			t.Errorf("Entries[%d] = %#v, want %#v", i, list.Entries[i], want[i])
		}
	}
}

func TestReadList_InvalidIDErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adminlist.txt")
	if err := os.WriteFile(path, []byte("not an id!\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := ReadList(path, domain.ListAdmin); err == nil {
		t.Fatalf("ReadList: want error for invalid id, got nil")
	}
}

func TestWriteList_ValidatesIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bannedlist.txt")
	err := WriteList(path, domain.PlayerList{
		Kind: domain.ListBanned,
		Entries: []domain.PlayerListEntry{
			{ID: "not valid!!"},
		},
	})
	if err == nil {
		t.Fatalf("WriteList: want validation error, got nil")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("Code = %q, want %q", de.Code, domain.CodeValidationFailed)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("file was created despite validation failure")
	}
}

func TestWriteList_RejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bannedlist.txt")
	err := WriteList(path, domain.PlayerList{
		Kind: domain.ListBanned,
		Entries: []domain.PlayerListEntry{
			{ID: "76561198000000001"},
			{ID: "76561198000000001"},
		},
	})
	if err == nil {
		t.Fatalf("WriteList: want validation error for duplicate id, got nil")
	}
}

func TestWriteList_CreatesFileAndIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "permittedlist.txt")
	err := WriteList(path, domain.PlayerList{
		Kind: domain.ListPermitted,
		Entries: []domain.PlayerListEntry{
			{ID: "76561198000000001"},
		},
	})
	if err != nil {
		t.Fatalf("WriteList: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", fi.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "permittedlist.txt" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestList_RoundTripPreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adminlist.txt")
	content := "// header comment, kept verbatim\n" +
		"76561198000000001 // Bjorn\n" +
		"// standalone note above this entry\n" +
		"76561198000000002\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	list, err := ReadList(path, domain.ListAdmin)
	if err != nil {
		t.Fatalf("ReadList: %v", err)
	}

	// Write the same entries back unchanged (as a PUT with no new comments
	// would, aside from what the API surfaced): the header and the
	// standalone note above 76561198000000002 must survive verbatim.
	if err := WriteList(path, *list); err != nil {
		t.Fatalf("WriteList: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(raw)
	for _, want := range []string{
		"// header comment, kept verbatim",
		"76561198000000001 // Bjorn",
		"// standalone note above this entry",
		"76561198000000002",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("round-tripped file missing %q; got:\n%s", want, got)
		}
	}

	// And parsing it again reproduces the same PlayerList.
	list2, err := ReadList(path, domain.ListAdmin)
	if err != nil {
		t.Fatalf("ReadList (2nd pass): %v", err)
	}
	if len(list2.Entries) != len(list.Entries) {
		t.Fatalf("2nd pass entries = %#v, want %#v", list2.Entries, list.Entries)
	}
	for i := range list.Entries {
		if list2.Entries[i] != list.Entries[i] {
			t.Errorf("2nd pass entries[%d] = %#v, want %#v", i, list2.Entries[i], list.Entries[i])
		}
	}
}

func TestList_PutReplacesEntriesButKeepsUnrelatedComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adminlist.txt")
	content := "// note for 1\n76561198000000001\n76561198000000002 // will be dropped\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Simulate a PUT that keeps id 1 (no explicit new comment: the old
	// standalone note above it should be preserved) and drops id 2.
	err := WriteList(path, domain.PlayerList{
		Kind:    domain.ListAdmin,
		Entries: []domain.PlayerListEntry{{ID: "76561198000000001"}},
	})
	if err != nil {
		t.Fatalf("WriteList: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "// note for 1") {
		t.Errorf("expected preserved comment for kept id; got:\n%s", got)
	}
	if strings.Contains(got, "76561198000000002") {
		t.Errorf("expected removed id to be gone; got:\n%s", got)
	}
}
