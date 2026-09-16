package managedenv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinaryPrefixPreservesOffsetsAndSuffixes(t *testing.T) {
	old := []byte("/long-placeholder")
	input := []byte("header\x00/long-placeholder/share:/long-placeholder/lib\x00TRAILER")
	want := []byte("header\x00/新 目录/share:/新 目录/lib")
	got, err := replaceBinaryPrefix(input, old, []byte("/新 目录"))
	if err != nil || len(got) != len(input) || !bytes.HasPrefix(got, want) || !bytes.HasSuffix(got, []byte("\x00TRAILER")) {
		t.Fatalf("bad relocation: %q, %v", got, err)
	}
	if !bytes.Contains(input, old) {
		t.Fatal("modified original download bytes")
	}
	if _, err := replaceBinaryPrefix(input, old, []byte(strings.Repeat("x", len(old)+1))); err == nil {
		t.Fatal("accepted a prefix that cannot fit")
	}
	if _, err := replaceBinaryPrefix([]byte("/long-placeholder/unterminated"), old, []byte("/new")); err == nil {
		t.Fatal("accepted unterminated binary string")
	}
}

func TestPrefixRelocationRejectsEscapesAndOversizedManifest(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "untouched")
	if err := os.WriteFile(outside, []byte("/placeholder/data"), 0600); err != nil {
		t.Fatal(err)
	}
	write := func(entries []prefixFixup) {
		t.Helper()
		data, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "awaken-prefixes.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := &Manager{}
	relative, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}
	write([]prefixFixup{{Path: relative, Placeholder: "/placeholder", Mode: "text"}})
	if m.relocatePrefixes(context.Background(), root) == nil {
		t.Fatal("accepted outside file")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "/placeholder/data" {
		t.Fatal("changed outside file")
	}
	if err := os.WriteFile(filepath.Join(root, "awaken-prefixes.json"), bytes.Repeat([]byte(" "), (8<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if m.relocatePrefixes(context.Background(), root) == nil {
		t.Fatal("accepted oversized manifest")
	}
}

func TestPrefixRelocationUsesFinalChineseDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "共享 组件")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var err error
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	old := "/" + strings.Repeat("placeholder", 24)
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte(old+"/share/poppler\x00TAIL"), 0600); err != nil {
		t.Fatal(err)
	}
	entries, _ := json.Marshal([]prefixFixup{{Path: "data.bin", Placeholder: old, Mode: "binary"}})
	if err := os.WriteFile(filepath.Join(root, "awaken-prefixes.json"), entries, 0600); err != nil {
		t.Fatal(err)
	}
	if err := (&Manager{}).relocatePrefixes(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "data.bin"))
	if err != nil || !bytes.HasPrefix(data, []byte(filepath.ToSlash(root)+"/share/poppler\x00")) {
		t.Fatalf("final path not used: %q %v", data, err)
	}
}
