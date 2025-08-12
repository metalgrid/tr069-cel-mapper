package loader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/example/cel-mapper/pkg/types"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	searchPaths []string
}

func New(searchPaths ...string) *Loader {
	return &Loader{
		searchPaths: searchPaths,
	}
}

func (l *Loader) AddSearchPath(path string) {
	l.searchPaths = append(l.searchPaths, path)
}

func (l *Loader) LoadFile(filename string) (*types.RulesConfig, error) {
	file, err := l.findFile(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return l.Load(file)
}

func (l *Loader) Load(r io.Reader) (*types.RulesConfig, error) {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)

	var config types.RulesConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	if err := l.validate(&config); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &config, nil
}

func (l *Loader) LoadString(content string) (*types.RulesConfig, error) {
	return l.Load(stringReader(content))
}

func (l *Loader) findFile(filename string) (*os.File, error) {
	if filepath.IsAbs(filename) {
		file, err := os.Open(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
		}
		return file, nil
	}

	var lastErr error
	for _, searchPath := range l.searchPaths {
		path := filepath.Join(searchPath, filename)
		file, err := os.Open(path)
		if err == nil {
			return file, nil
		}
		lastErr = err
	}

	file, err := os.Open(filename)
	if err != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("file not found in search paths or current directory: %s", filename)
		}
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	return file, nil
}

func (l *Loader) validate(config *types.RulesConfig) error {
	if config.Version == "" {
		return fmt.Errorf("version is required")
	}

	if len(config.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	seenNames := make(map[string]bool)
	for i, rule := range config.Rules {
		if rule.Name == "" {
			return fmt.Errorf("rule[%d]: name is required", i)
		}
		if seenNames[rule.Name] {
			return fmt.Errorf("rule[%d]: duplicate rule name: %s", i, rule.Name)
		}
		seenNames[rule.Name] = true

		if rule.Target == "" {
			return fmt.Errorf("rule[%d] %s: target is required", i, rule.Name)
		}
		if rule.Route == "" {
			return fmt.Errorf("rule[%d] %s: route expression is required", i, rule.Name)
		}
		if rule.EntityKey == "" {
			return fmt.Errorf("rule[%d] %s: entity_key expression is required", i, rule.Name)
		}

		for j, field := range rule.Fields {
			if field.Name == "" {
				return fmt.Errorf("rule[%d] %s field[%d]: name is required", i, rule.Name, j)
			}
			if field.When == "" {
				return fmt.Errorf("rule[%d] %s field[%d] %s: when expression is required", i, rule.Name, j, field.Name)
			}
			if field.Value == "" {
				return fmt.Errorf("rule[%d] %s field[%d] %s: value expression is required", i, rule.Name, j, field.Name)
			}
		}
	}

	return nil
}

func LoadFile(filename string) (*types.RulesConfig, error) {
	loader := New()
	return loader.LoadFile(filename)
}

func LoadString(content string) (*types.RulesConfig, error) {
	loader := New()
	return loader.LoadString(content)
}

type stringReader string

func (s stringReader) Read(p []byte) (n int, err error) {
	n = copy(p, s)
	if n < len(s) {
		return n, nil
	}
	return n, io.EOF
}
