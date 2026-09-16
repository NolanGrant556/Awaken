package managedenv

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (Options, *atomic.Int32) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture; Windows tests are compiled separately")
	}
	var count atomic.Int32
	body := []byte("#!/bin/sh\nprintf 'fixture 1.0.0\\n'\n")
	hash := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); _, _ = w.Write(body) }))
	t.Cleanup(server.Close)
	c := Catalog{SchemaVersion: SchemaVersion, Revision: "test", Components: map[string]Component{"fixture": {Version: "1.0.0", Probe: "fixture", ProbeArgs: []string{"--version"}, Assets: map[string]Asset{Target(): {URL: server.URL, SHA256: hex.EncodeToString(hash[:]), Format: "binary", Executables: map[string]string{"fixture": "bin/fixture"}}}}}, Capabilities: map[string]Spec{"fixture": {Components: []string{"fixture"}}}}
	return Options{Root: t.TempDir(), Catalog: c, Client: server.Client(), AllowFileAssets: true}, &count
}
func managerForTest(t *testing.T, o Options) *Manager {
	t.Helper()
	m, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}
func testOwner(s string) Owner {
	return Owner{ThreadID: s, AgentID: "root", RunID: s + "-run", Kind: "run"}
}

func TestCrossProjectRestartAndIndexRecoveryReuse(t *testing.T) {
	o, count := fixture(t)
	m := managerForTest(t, o)
	b, e := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if b.Reused || count.Load() != 1 {
		t.Fatalf("first prepare: %+v downloads=%d", b, count.Load())
	}
	second, e := m.Ensure(context.Background(), Request{Requirements: []Requirement{{Ecosystem: "component", Name: "fixture"}}}, testOwner("B"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if !second.Reused || b.InstanceID != second.InstanceID || count.Load() != 1 {
		t.Fatal("cross-project installation was duplicated")
	}
	if _, e = m.Binding(context.Background(), b.EnvironmentRef, testOwner("B")); e == nil {
		t.Fatal("foreign reference accepted")
	}
	if e = m.Close(); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(m.root, "registry.json")); e != nil {
		t.Fatal(e)
	}
	restarted := managerForTest(t, o)
	again, e := restarted.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if !again.Reused || again.InstanceID != b.InstanceID || count.Load() != 1 {
		t.Fatal("restart/index recovery reinstalled a healthy environment")
	}
}
func TestConcurrentManagersPrepareOnce(t *testing.T) {
	o, count := fixture(t)
	a := managerForTest(t, o)
	b := managerForTest(t, o)
	var wg sync.WaitGroup
	results := make(chan Binding, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := a
			if i%2 == 0 {
				m = b
			}
			value, e := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner(string(rune('A'+i))), nil)
			if e != nil {
				errs <- e
			} else {
				results <- value
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	close(results)
	for e := range errs {
		t.Error(e)
	}
	id := ""
	for value := range results {
		if id != "" && id != value.InstanceID {
			t.Error("multiple physical instances")
		}
		id = value.InstanceID
	}
	if count.Load() != 1 {
		t.Fatalf("downloads=%d", count.Load())
	}
}
func TestCancellingOneWaiterDoesNotCancelOthers(t *testing.T) {
	o, _ := fixture(t)
	started := make(chan struct{})
	finish := make(chan struct{})
	var once sync.Once
	original := o.Client.Transport
	o.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		select {
		case <-finish:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		if original == nil {
			return http.DefaultTransport.RoundTrip(r)
		}
		return original.RoundTrip(r)
	})}
	m := managerForTest(t, o)
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, e := m.Ensure(ctx, Request{Capability: "fixture"}, testOwner("first"), nil); first <- e }()
	<-started
	second := make(chan error, 1)
	go func() {
		_, e := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("second"), nil)
		second <- e
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		paths, _ := filepath.Glob(filepath.Join(m.root, "waiters", "*", "*.json"))
		if len(paths) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if e := <-first; e != context.Canceled {
		t.Fatalf("cancel=%v", e)
	}
	close(finish)
	if e := <-second; e != nil {
		t.Fatal(e)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestChecksumFailureDoesNotPublishReady(t *testing.T) {
	o, _ := fixture(t)
	c := o.Catalog.Components["fixture"]
	a := c.Assets[Target()]
	a.SHA256 = strings.Repeat("a", 64)
	c.Assets[Target()] = a
	o.Catalog.Components["fixture"] = c
	m := managerForTest(t, o)
	_, e := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
	if e == nil || !strings.Contains(e.Error(), "checksum") {
		t.Fatalf("expected checksum failure: %v", e)
	}
	all, _ := m.instances()
	if len(all) != 0 {
		t.Fatal("failed download published instances")
	}
}
func TestSchemaAndRootRejection(t *testing.T) {
	o, _ := fixture(t)
	o.Root = "relative"
	if _, e := New(o); e == nil {
		t.Fatal("relative root accepted")
	}
	o.Root = t.TempDir()
	root := filepath.Join(o.Root, "runtime", "environments")
	if e := writeJSON(filepath.Join(root, "registry.json"), map[string]any{"schema_version": 999}); e != nil {
		t.Fatal(e)
	}
	if _, e := New(o); e == nil {
		t.Fatal("unknown schema accepted")
	}
	var raw map[string]any
	_ = readJSON(filepath.Join(root, "registry.json"), &raw)
	if raw["schema_version"] != float64(999) {
		t.Fatal("unknown schema overwritten")
	}
}
func TestCorruptIndexPreservesEvidence(t *testing.T) {
	o, _ := fixture(t)
	m := managerForTest(t, o)
	b, e := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(m.root, "registry.json"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	again, e := m.Resolve(context.Background(), Request{Capability: "fixture"})
	if e != nil || again.InstanceID != b.InstanceID {
		t.Fatalf("recover: %v", e)
	}
	paths, _ := filepath.Glob(filepath.Join(m.root, "registry.json.invalid-*"))
	if len(paths) != 1 {
		t.Fatal("corrupt evidence lost")
	}
}
func TestArchiveRejectsTraversalAndLinks(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/absolute"} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			tw := tar.NewWriter(&b)
			_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: 1})
			_, _ = tw.Write([]byte("x"))
			_ = tw.Close()
			path := filepath.Join(t.TempDir(), "asset.tar")
			_ = os.WriteFile(path, b.Bytes(), 0600)
			if e := extract(context.Background(), path, t.TempDir(), Asset{Format: "tar"}); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestPackageNamesCannotBecomeCommands(t *testing.T) {
	o, _ := fixture(t)
	for _, req := range []Requirement{{"python", "x;touch /tmp/bad", ""}, {"node", "file:/tmp/x", ""}, {"python", "foo", ">=1;id"}, {"component", "../x", ""}} {
		if _, e := o.Catalog.Resolve(Request{Requirements: []Requirement{req}}); e == nil {
			t.Fatalf("unsafe requirement accepted: %+v", req)
		}
	}
}
func TestEnvironmentDoesNotPolluteBaseOrExposeSecrets(t *testing.T) {
	b := Binding{EnvironmentRef: "ref", Executables: map[string]Launch{"python": {Executable: "/managed/bin/python"}}, Environment: map[string]string{"VIRTUAL_ENV": "/managed"}}
	base := []string{"PATH=/base", "PYTHONPATH=/wrong", "NODE_OPTIONS=--inspect", "BUSINESS_TOKEN=keep-for-business"}
	out := b.ExecutionEnvironment(base)
	encoded := strings.Join(out, "\n")
	if strings.Contains(encoded, "/wrong") || strings.Contains(encoded, "--inspect") || !strings.Contains(encoded, "BUSINESS_TOKEN=keep-for-business") {
		t.Fatal(encoded)
	}
	if base[1] != "PYTHONPATH=/wrong" {
		t.Fatal("base environment mutated")
	}
	raw, _ := json.Marshal(b)
	if strings.Contains(string(raw), "BUSINESS_TOKEN") {
		t.Fatal("binding leaked process environment")
	}
}
func TestVersionConstraints(t *testing.T) {
	for _, tc := range []struct {
		version, constraint string
		want                bool
	}{{"2.3.3", ">=2,<3", true}, {"3.0.0", ">=2,<3", false}, {"1.2.3", "^1.1", true}, {"2.0.0", "^1.1", false}, {"1.2.3", "1.2.*", true}, {"1.3.0", "1.2.*", false}, {"1.2.3", "2.0.0", false}} {
		if got := satisfies(tc.version, tc.constraint); got != tc.want {
			t.Errorf("%s %s=%v", tc.version, tc.constraint, got)
		}
	}
}

func TestArchiveResolvesFrameworkLinkDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("framework symlinks are macOS resource layout")
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for _, header := range []*tar.Header{
		{Name: "Framework/Resources", Typeflag: tar.TypeSymlink, Linkname: "Versions/Current/Resources", Mode: 0777},
		{Name: "Framework/Versions/Current", Typeflag: tar.TypeSymlink, Linkname: "3.12", Mode: 0777},
		{Name: "Framework/Versions/3.12/Resources/marker", Typeflag: tar.TypeReg, Mode: 0600, Size: 2},
	} {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			_, _ = writer.Write([]byte("ok"))
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "framework.tar")
	if err := os.WriteFile(source, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extract(context.Background(), source, dest, Asset{Format: "tar"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "Framework/Resources/marker"))
	if err != nil || string(content) != "ok" {
		t.Fatalf("framework link invalid: %s %v", content, err)
	}
}
