package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Drift watcher for issue #72 (precedent PR #64/#67): the model label in the
// SPA must come from the observability stats of the serving gateway, never
// from a constant. Two literals encode the old lie: the hardcoded display
// name "Gemma 12B" and reads of healthData.ai_model — /api/v1/health
// deliberately never returns ai_model (finding F-40), so any UI reference to
// it silently resurrects the fake fallback label. If either reappears under
// web/src, the truthful wiring was replaced by a constant again.
func TestWebUIHasNoHardcodedModelLabels(t *testing.T) {
	forbidden := []string{"Gemma 12B", "ai_model"}

	root := filepath.Join("..", "..", "web", "src")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".ts" && ext != ".tsx" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, lit := range forbidden {
			if strings.Contains(string(content), lit) {
				t.Errorf("%s contains hardcoded model label %q — read stats.active_model from /api/v1/ai/observability (hooks/useAIModel) instead", path, lit)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scanning %s: %v", root, err)
	}
}
