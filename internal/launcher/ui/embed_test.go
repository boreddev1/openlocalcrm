package ui

import (
	"io/fs"
	"testing"
)

func TestUIFilesExist(t *testing.T) {
	requiredFiles := []string{"index.html", "style.css", "app.js"}
	for _, file := range requiredFiles {
		if _, err := fs.Stat(DistFS, file); err != nil {
			t.Errorf("required file %q missing from embedded DistFS: %v", file, err)
		}
	}
}
