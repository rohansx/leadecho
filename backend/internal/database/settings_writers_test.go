package database

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workspaces.settings holds keys that other processes write on their own
// (the first-leads email flag, the daily company-scan claim, a requested
// scan). A handler that reads the whole object, changes its key and writes
// the whole object back drops whatever was written in between: the email is
// sent twice, the paid scan runs twice. Writers change their own key with
// SetWorkspaceSettingsKey or SwapWorkspaceSettingsKey.
func TestNobodyRewritesTheWholeSettingsObject(t *testing.T) {
	for _, root := range []string{"..", filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(filepath.Dir(path)) == "database" {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(src), ".UpdateWorkspaceSettings(") {
				t.Errorf("%s rewrites the whole settings object: use SetWorkspaceSettingsKey", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
