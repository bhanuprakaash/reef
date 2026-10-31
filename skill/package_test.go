package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFolderRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFolder(dir)
	if err == nil || !strings.Contains(err.Error(), "link is a symlink") {
		t.Errorf("ReadFolder with a symlink = %v, want a symlink error", err)
	}
}
