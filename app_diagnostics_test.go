package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.2.0", "1.1.9", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.10", "1.9", 1},
		{"2.0.0-beta.1", "2.0.0", 0},
		{"", "0.0.1", -1},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestTargetExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if !targetExists(file) {
		t.Errorf("targetExists(existing file) = false")
	}
	if targetExists(filepath.Join(dir, "missing.exe")) {
		t.Errorf("targetExists(missing file) = true")
	}
	if !targetExists("https://example.com") {
		t.Errorf("targetExists(uri) = false")
	}
	if targetExists("   ") {
		t.Errorf("targetExists(blank) = true")
	}
}

func TestIconBaseName(t *testing.T) {
	cases := map[string]string{
		"icons/abc.png":  "abc.png",
		"icons/sub/a.p":  "a.p",
		"C:/icons/x.png": "",
		"":               "",
	}
	for in, want := range cases {
		if got := iconBaseName(in); got != want {
			t.Errorf("iconBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSingleInstanceNamesDependOnBaseDir(t *testing.T) {
	mutexA, classA, windowA := SingleInstanceNames(`F:\Softwares\JeeDesk`)
	mutexB, classB, windowB := SingleInstanceNames(`f:\softwares\jeedesk`)
	if mutexA != mutexB || classA != classB || windowA != windowB {
		t.Errorf("names should be case insensitive: %q/%q %q/%q %q/%q", mutexA, mutexB, classA, classB, windowA, windowB)
	}

	mutexC, _, _ := SingleInstanceNames(`F:\Portable\JeeDesk`)
	if mutexA == mutexC {
		t.Errorf("different data directories must not share a mutex name")
	}
}
