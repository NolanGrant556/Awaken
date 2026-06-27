package mcpresources

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Client struct {
	Name              string
	Connected         bool
	SupportsResources bool
	Resources         []ResourceDescriptor
}

type ResourceDescriptor struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	MIMEType    string `json:"mimeType,omitempty"`
	Description string `json:"description,omitempty"`
	Server      string `json:"server"`
}

type ReadResult struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	URI      string  `json:"uri"`
	MIMEType string  `json:"mimeType,omitempty"`
	Text     *string `json:"text,omitempty"`
	Blob     *string `json:"blob,omitempty"`
}

type NormalizedReadResult struct {
	Contents []NormalizedContent `json:"contents"`
}

type NormalizedContent struct {
	URI         string `json:"uri"`
	MIMEType    string `json:"mimeType,omitempty"`
	Text        string `json:"text,omitempty"`
	BlobSavedTo string `json:"blobSavedTo,omitempty"`
}

type PersistRequest struct {
	Bytes     []byte
	MIMEType  string
	Extension string
	Server    string
	URI       string
	Index     int
}

type PersistFunc func(PersistRequest) (string, error)

func List(clients []Client, targetServer string) ([]ResourceDescriptor, error) {
	selected, err := clientsForList(clients, targetServer)
	if err != nil {
		return nil, err
	}

	resources := make([]ResourceDescriptor, 0)
	for _, client := range selected {
		if !client.Connected || !client.SupportsResources {
			continue
		}
		for _, resource := range client.Resources {
			descriptor := resource
			descriptor.Server = client.Name
			resources = append(resources, descriptor)
		}
	}

	sort.SliceStable(resources, func(i, j int) bool {
		a, b := resources[i], resources[j]
		return compareResource(a, b) < 0
	})
	return resources, nil
}

func Read(clients []Client, server, uri string, reader func(Client, string) (ReadResult, error), persist PersistFunc) (NormalizedReadResult, error) {
	client, ok := findClient(clients, server)
	if !ok {
		return NormalizedReadResult{}, serverNotFoundError(server, clients)
	}
	if !client.Connected {
		return NormalizedReadResult{}, fmt.Errorf("server %q is not connected", server)
	}
	if !client.SupportsResources {
		return NormalizedReadResult{}, fmt.Errorf("server %q does not support resources", server)
	}
	if reader == nil {
		return NormalizedReadResult{}, errors.New("reader is nil")
	}

	result, err := reader(client, uri)
	if err != nil {
		return NormalizedReadResult{}, err
	}

	normalized := NormalizedReadResult{
		Contents: make([]NormalizedContent, 0, len(result.Contents)),
	}
	for i, content := range result.Contents {
		normalized.Contents = append(normalized.Contents, normalizeContent(client.Name, content, i, persist))
	}
	return normalized, nil
}

func ExtensionForMIMEType(mimeType string) string {
	if mimeType == "" {
		return "bin"
	}

	mt := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch mt {
	case "application/json":
		return "json"
	case "application/pdf":
		return "pdf"
	case "application/zip":
		return "zip"
	case "image/gif":
		return "gif"
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/svg+xml":
		return "svg"
	case "image/webp":
		return "webp"
	case "text/csv":
		return "csv"
	case "text/html":
		return "html"
	case "text/markdown":
		return "md"
	case "text/plain":
		return "txt"
	default:
		return "bin"
	}
}

func clientsForList(clients []Client, targetServer string) ([]Client, error) {
	if targetServer == "" {
		return clients, nil
	}

	for _, client := range clients {
		if client.Name == targetServer {
			return []Client{client}, nil
		}
	}
	return nil, serverNotFoundError(targetServer, clients)
}

func findClient(clients []Client, server string) (Client, bool) {
	for _, client := range clients {
		if client.Name == server {
			return client, true
		}
	}
	return Client{}, false
}

func serverNotFoundError(server string, clients []Client) error {
	names := make([]string, 0, len(clients))
	for _, client := range clients {
		names = append(names, client.Name)
	}
	sort.Strings(names)
	return fmt.Errorf("server %q not found. available servers: %s", server, strings.Join(names, ", "))
}

func compareResource(a, b ResourceDescriptor) int {
	for _, pair := range [][2]string{
		{a.Server, b.Server},
		{a.URI, b.URI},
		{a.Name, b.Name},
		{a.MIMEType, b.MIMEType},
		{a.Description, b.Description},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func normalizeContent(server string, content Content, index int, persist PersistFunc) NormalizedContent {
	normalized := NormalizedContent{
		URI:      content.URI,
		MIMEType: content.MIMEType,
	}
	if content.Text != nil {
		normalized.Text = *content.Text
		return normalized
	}
	if content.Blob == nil {
		return normalized
	}

	bytes, err := base64.StdEncoding.DecodeString(*content.Blob)
	if err != nil {
		normalized.Text = fmt.Sprintf("Binary content could not be decoded from base64: %v", err)
		return normalized
	}
	if persist == nil {
		normalized.Text = "Binary content could not be saved to disk: persist function is nil"
		return normalized
	}

	path, err := persist(PersistRequest{
		Bytes:     bytes,
		MIMEType:  content.MIMEType,
		Extension: ExtensionForMIMEType(content.MIMEType),
		Server:    server,
		URI:       content.URI,
		Index:     index,
	})
	if err != nil {
		normalized.Text = fmt.Sprintf("Binary content could not be saved to disk: %v", err)
		return normalized
	}

	normalized.BlobSavedTo = path
	normalized.Text = binaryBlobSavedMessage(server, content.URI, content.MIMEType, len(bytes), path)
	return normalized
}

func binaryBlobSavedMessage(server, uri, mimeType string, size int, path string) string {
	mt := mimeType
	if mt == "" {
		mt = "unknown type"
	}
	return fmt.Sprintf("[Resource from %s at %s] Binary content (%s, %d bytes) saved to %s", server, uri, mt, size, path)
}
