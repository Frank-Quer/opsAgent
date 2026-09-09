package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplacementAndCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("%q %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("file permissions")
	}
	blocked := filepath.Join(dir, "directory")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(blocked, []byte("replacement")); err == nil {
		t.Fatal("replaced directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("temporary files remain: %v %v", entries, err)
	}
	info, err = os.Stat(blocked)
	if err != nil || !info.IsDir() {
		t.Fatal("failed replacement changed destination")
	}
}
