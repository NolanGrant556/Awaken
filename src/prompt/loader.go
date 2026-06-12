package prompt

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed templates/default_system.md
var embeddedDefaultSystemPrompt string

type Loader struct {
	templateDir string
}

func NewLoader(templateDir string) *Loader {
	return &Loader{
		templateDir: templateDir,
	}
}

func (l *Loader) Load(templateName string) (string, error) {
	path := filepath.Join(l.templateDir, templateName)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (l *Loader) MustLoad(templateName string) string {
	content, err := l.Load(templateName)
	if err != nil {
		panic(err)
	}
	return content
}

func LoadDefaultPrompt() (string, error) {
	for _, dir := range defaultTemplateDirs() {
		content, err := NewLoader(dir).Load("default_system.md")
		if err == nil {
			return content, nil
		}
	}
	return embeddedDefaultSystemPrompt, nil
}

func defaultTemplateDirs() []string {
	dirs := []string{"src/prompt/templates", "templates"}
	wd, err := os.Getwd()
	if err != nil {
		return dirs
	}

	for dir := wd; ; dir = filepath.Dir(dir) {
		dirs = append(dirs, filepath.Join(dir, "src/prompt/templates"))
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return dirs
}
