package mcpresources

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestListReturnsErrorForMissingTargetServer(t *testing.T) {
	_, err := List([]Client{{Name: "alpha"}}, "beta")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `server "beta" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadReturnsErrorForMissingServer(t *testing.T) {
	_, err := Read([]Client{{Name: "alpha"}}, "beta", "file://missing", stubReader(ReadResult{}), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `server "beta" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadReturnsErrorForDisconnectedServer(t *testing.T) {
	_, err := Read([]Client{
		{Name: "alpha", Connected: false, SupportsResources: true},
	}, "alpha", "file://note", stubReader(ReadResult{}), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `server "alpha" is not connected`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadReturnsErrorWhenServerDoesNotSupportResources(t *testing.T) {
	_, err := Read([]Client{
		{Name: "alpha", Connected: true, SupportsResources: false},
	}, "alpha", "file://note", stubReader(ReadResult{}), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `server "alpha" does not support resources`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadReturnsTextContentUnchanged(t *testing.T) {
	text := "hello\n\nraw text"
	result, err := Read([]Client{
		{Name: "alpha", Connected: true, SupportsResources: true},
	}, "alpha", "file://note", stubReader(ReadResult{
		Contents: []Content{{URI: "file://note", MIMEType: "text/plain", Text: &text}},
	}), nil)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected one content, got %d", len(result.Contents))
	}
	if result.Contents[0].Text != text {
		t.Fatalf("expected text to be unchanged, got %q", result.Contents[0].Text)
	}
	if result.Contents[0].BlobSavedTo != "" {
		t.Fatalf("expected no saved blob path, got %q", result.Contents[0].BlobSavedTo)
	}
}

func TestReadSavesBlobContent(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString([]byte("hello"))
	var persisted PersistRequest
	result, err := Read([]Client{
		{Name: "alpha", Connected: true, SupportsResources: true},
	}, "alpha", "mcp://image", stubReader(ReadResult{
		Contents: []Content{{URI: "mcp://image", MIMEType: "image/png", Blob: &blob}},
	}), func(request PersistRequest) (string, error) {
		persisted = request
		return "/tmp/mcp-resource.png", nil
	})
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	if string(persisted.Bytes) != "hello" {
		t.Fatalf("unexpected persisted bytes: %q", string(persisted.Bytes))
	}
	if persisted.MIMEType != "image/png" || persisted.Extension != "png" || persisted.Server != "alpha" || persisted.URI != "mcp://image" {
		t.Fatalf("unexpected persist request: %+v", persisted)
	}
	if result.Contents[0].BlobSavedTo != "/tmp/mcp-resource.png" {
		t.Fatalf("unexpected blob path: %+v", result.Contents[0])
	}
	message := result.Contents[0].Text
	for _, expected := range []string{"alpha", "mcp://image", "image/png", "5 bytes", "/tmp/mcp-resource.png"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("expected message %q to contain %q", message, expected)
		}
	}
}

func TestReadReportsInvalidBase64AndPersistFailureWithoutAborting(t *testing.T) {
	invalidBlob := "not base64!"
	validBlob := base64.StdEncoding.EncodeToString([]byte("ok"))
	result, err := Read([]Client{
		{Name: "alpha", Connected: true, SupportsResources: true},
	}, "alpha", "mcp://mixed", stubReader(ReadResult{
		Contents: []Content{
			{URI: "mcp://bad", MIMEType: "application/octet-stream", Blob: &invalidBlob},
			{URI: "mcp://save-fails", MIMEType: "application/pdf", Blob: &validBlob},
		},
	}), func(PersistRequest) (string, error) {
		return "", errors.New("disk full")
	})
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if len(result.Contents) != 2 {
		t.Fatalf("expected two contents, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "could not be decoded from base64") {
		t.Fatalf("unexpected invalid base64 text: %+v", result.Contents[0])
	}
	if result.Contents[0].BlobSavedTo != "" {
		t.Fatalf("expected invalid blob not to be saved: %+v", result.Contents[0])
	}
	if !strings.Contains(result.Contents[1].Text, "could not be saved to disk: disk full") {
		t.Fatalf("unexpected persist failure text: %+v", result.Contents[1])
	}
	if result.Contents[1].BlobSavedTo != "" {
		t.Fatalf("expected failed persist not to set path: %+v", result.Contents[1])
	}
}

func TestListFiltersTargetServerAddsServerAndSorts(t *testing.T) {
	resources, err := List([]Client{
		{
			Name:              "alpha",
			Connected:         true,
			SupportsResources: true,
			Resources: []ResourceDescriptor{
				{URI: "mcp://alpha/z", Name: "zeta"},
			},
		},
		{
			Name:              "beta",
			Connected:         true,
			SupportsResources: true,
			Resources: []ResourceDescriptor{
				{URI: "mcp://beta/z", Name: "zeta", Server: "wrong"},
				{URI: "mcp://beta/a", Name: "alpha"},
			},
		},
		{
			Name:              "beta-disconnected",
			Connected:         false,
			SupportsResources: true,
			Resources: []ResourceDescriptor{
				{URI: "mcp://beta-disconnected/a", Name: "hidden"},
			},
		},
	}, "beta")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected two beta resources, got %+v", resources)
	}
	if resources[0].URI != "mcp://beta/a" || resources[1].URI != "mcp://beta/z" {
		t.Fatalf("expected resources sorted by URI, got %+v", resources)
	}
	for _, resource := range resources {
		if resource.Server != "beta" {
			t.Fatalf("expected server to be set to beta, got %+v", resource)
		}
	}
}

func TestListSkipsDisconnectedClients(t *testing.T) {
	resources, err := List([]Client{
		{
			Name:              "alpha",
			Connected:         false,
			SupportsResources: true,
			Resources: []ResourceDescriptor{
				{URI: "mcp://alpha/a", Name: "hidden"},
			},
		},
		{
			Name:              "beta",
			Connected:         true,
			SupportsResources: true,
			Resources: []ResourceDescriptor{
				{URI: "mcp://beta/a", Name: "visible"},
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected only connected resources, got %+v", resources)
	}
	if resources[0].Server != "beta" || resources[0].URI != "mcp://beta/a" {
		t.Fatalf("unexpected resource: %+v", resources[0])
	}
}

func stubReader(result ReadResult) func(Client, string) (ReadResult, error) {
	return func(Client, string) (ReadResult, error) {
		return result, nil
	}
}
