package backup

import "testing"

func TestIsValheimBackupStem(t *testing.T) {
	cases := map[string]bool{
		"Midgard":        false,
		"Dedotated wham": false,
		"Dedotated wham_backup_auto-20260909144803": true,
		"Midgard_backup_cloud-20260101000000":       true,
		"Midgard_backup_restore-20260101000000":     true,
		"backup_auto":                               false,
	}
	for stem, want := range cases {
		if got := isValheimBackupStem(stem); got != want {
			t.Errorf("%q: got %v want %v", stem, got, want)
		}
	}
}
