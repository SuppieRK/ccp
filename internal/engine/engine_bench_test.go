package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkEnginePipeline(b *testing.B) {
	cases := []struct {
		name    string
		fixture string
	}{
		{name: "NoiseBomb", fixture: "noise-bomb.log"},
		{name: "RaceCondition", fixture: "race-condition.log"},
		{name: "DeepJSON", fixture: "deep-json.log"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			path := filepath.Join("..", "..", "testdata", "fixtures", tc.fixture)
			data, err := os.ReadFile(path)
			if err != nil {
				b.Fatalf("read fixture: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e := NewEngine(Config{
					NeverDropPatterns: nil,
					Filters: []ToolFilter{fixtureCollectFilter{
						engineTestFilterBase: engineTestFilterBase{tool: "fixture", sharedContext: true},
					}},
				})
				e.SetCommandID("bench")
				for _, line := range lines {
					_ = e.Process(string(StdoutStream), "fixture", Input{Line: line + "\n"})
				}
				_ = e.Process(string(StdoutStream), "fixture", Input{EOF: true})
			}
		})
	}
}
