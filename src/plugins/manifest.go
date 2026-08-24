package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var allowedManifestFields = map[string]bool{
	"$schema":     true,
	"name":        true,
	"version":     true,
	"description": true,
	"author":      true,
	"homepage":    true,
	"repository":  true,
	"license":     true,
	"keywords":    true,
	"extensions":  true,
}

type manifestParseResult struct {
	Manifest    Manifest
	Diagnostics []Diagnostic
}

func loadManifestFile(pluginRoot string) (manifestParseResult, error) {
	path := filepath.Join(pluginRoot, "plugin.json")
	if err := rejectSymlink(path); err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if !isRegularFile(path) {
		return manifestParseResult{}, fmt.Errorf("%w: plugin.json is missing or not a regular file", ErrManifestInvalid)
	}
	if err := resolvedStayWithin(pluginRoot, path); err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: plugin.json escapes the plugin root", ErrManifestInvalid)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: read plugin.json: %v", ErrManifestInvalid, err)
	}
	return parseManifest(raw)
}

func parseManifest(raw []byte) (manifestParseResult, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: plugin.json is not valid JSON", ErrManifestInvalid)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return manifestParseResult{}, fmt.Errorf("%w: plugin.json must contain a JSON object", ErrManifestInvalid)
	}

	var diags []Diagnostic
	for field := range object {
		if !allowedManifestFields[field] {
			diags = append(diags, warning("plugin_unknown_field", "ignoring unknown plugin.json field "+field))
			delete(object, field)
		}
	}
	if ext, exists := object["extensions"]; exists {
		if _, isObject := ext.(map[string]any); !isObject {
			diags = append(diags, warning("plugin_extensions_ignored", "ignoring non-object plugin.json extensions field"))
			delete(object, extKey)
		}
	}

	schema, err := requiredString(object, "$schema")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if schema != SchemaPluginV1 {
		if strings.HasPrefix(schema, SchemaPrefix) {
			return manifestParseResult{}, fmt.Errorf("%w: unsupported Agent Plugins schema %q", ErrUnsupportedSchema, schema)
		}
		return manifestParseResult{}, fmt.Errorf("%w: plugin.json is not an Agent Plugins manifest", ErrUnsupportedSchema)
	}

	name, err := requiredString(object, "name")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if !isValidPluginName(name) {
		return manifestParseResult{}, fmt.Errorf("%w: invalid plugin name %q", ErrManifestInvalid, name)
	}

	version, err := optionalString(object, "version")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	description, err := optionalString(object, "description")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	homepage, err := optionalString(object, "homepage")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	repository, err := optionalString(object, "repository")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	license, err := optionalString(object, "license")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	keywords, err := optionalStringArray(object, "keywords")
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	author, err := optionalAuthor(object["author"])
	if err != nil {
		return manifestParseResult{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}

	return manifestParseResult{
		Manifest: Manifest{
			Schema:      schema,
			Name:        name,
			Version:     version,
			Description: description,
			Author:      author,
			Homepage:    homepage,
			Repository:  repository,
			License:     license,
			Keywords:    keywords,
		},
		Diagnostics: diags,
	}, nil
}

const extKey = "extensions"

func requiredString(object map[string]any, field string) (string, error) {
	raw, ok := object[field]
	if !ok {
		return "", fmt.Errorf("%s is required", field)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", field)
	}
	return value, nil
}

func optionalString(object map[string]any, field string) (string, error) {
	raw, ok := object[field]
	if !ok {
		return "", nil
	}
	if raw == nil {
		return "", fmt.Errorf("%s must use its declared type when present", field)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return strings.TrimSpace(value), nil
}

func optionalStringArray(object map[string]any, field string) ([]string, error) {
	raw, ok := object[field]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", field)
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", field, i)
		}
		out = append(out, value)
	}
	return out, nil
}

func optionalAuthor(raw any) (*Author, error) {
	if raw == nil {
		return nil, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("author must be an object")
	}
	author := &Author{}
	for field, value := range object {
		switch field {
		case "name", "email", "url":
			if value == nil {
				return nil, fmt.Errorf("author.%s must be a string when present", field)
			}
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("author.%s must be a string", field)
			}
			text = strings.TrimSpace(text)
			switch field {
			case "name":
				author.Name = text
			case "email":
				author.Email = text
			case "url":
				author.URL = text
			}
		default:
			return nil, fmt.Errorf("author contains unknown field %s", field)
		}
	}
	if author.Name == "" && author.Email == "" && author.URL == "" {
		return nil, nil
	}
	return author, nil
}
