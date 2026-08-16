package filtermappings

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

type file struct {
	Version int               `yaml:"version"`
	Map     map[string]string `yaml:"map"`
}

// Decode parses the single supported mappings document and returns a
// normalized copy. Callers can safely mutate the returned map.
func Decode(path string, raw []byte) (map[string]string, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var payload file
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode mappings %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple YAML documents")
		}
		return nil, fmt.Errorf("decode mappings %s: %w", path, err)
	}
	if payload.Version != 1 {
		return nil, fmt.Errorf("decode mappings %s: version must be exactly 1", path)
	}

	out := make(map[string]string, len(payload.Map))
	for alias, target := range payload.Map {
		alias = strings.TrimSpace(alias)
		target = strings.TrimSpace(target)
		if alias == "" || target == "" {
			return nil, fmt.Errorf("decode mappings %s: mapping keys and values must be non-empty", path)
		}
		if _, exists := out[alias]; exists {
			return nil, fmt.Errorf("decode mappings %s: duplicate normalized mapping %q", path, alias)
		}
		out[alias] = target
	}
	return out, nil
}
