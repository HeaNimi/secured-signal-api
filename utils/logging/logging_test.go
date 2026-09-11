package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileLogRotatesBySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.log")

	if err := ConfigureFile(path, 10, 2); err != nil {
		t.Fatal(err)
	}
	defer CloseFile()

	writeFileLog("12345")
	writeFileLog("67890")

	active, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}

	if string(rotated) != "12345\n" {
		t.Fatalf("rotated log = %q, want %q", rotated, "12345\\n")
	}
	if string(active) != "67890\n" {
		t.Fatalf("active log = %q, want %q", active, "67890\\n")
	}
}

func TestWriteFileLogRetainsConfiguredBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.log")

	if err := ConfigureFile(path, 1, 2); err != nil {
		t.Fatal(err)
	}
	defer CloseFile()

	for _, message := range []string{"one", "two", "three", "four"} {
		writeFileLog(message)
	}

	for _, suffix := range []string{".1", ".2"} {
		if _, err := os.Stat(path + suffix); err != nil {
			t.Fatalf("expected retained backup %s: %v", suffix, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup .3: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), filepath.Base(path)+".") && entry.Name() != filepath.Base(path)+".1" && entry.Name() != filepath.Base(path)+".2" {
			t.Fatalf("unexpected rotated file %s", entry.Name())
		}
	}
}
