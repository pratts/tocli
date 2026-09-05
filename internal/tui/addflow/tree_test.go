package addflow

import (
	"strings"
	"testing"

	"github.com/pratts/tocli/internal/engine"
)

func TestRenderFileTree_GroupsByDirectory(t *testing.T) {
	files := []engine.FileSummary{
		{Path: "Season1/ep1.mkv", Length: 100},
		{Path: "Season1/ep2.mkv", Length: 200},
		{Path: "Season2/ep1.mkv", Length: 150},
		{Path: "readme.txt", Length: 10},
	}

	tree := renderFileTree(files)

	for _, want := range []string{"Season1/", "Season2/", "ep1.mkv", "ep2.mkv", "readme.txt"} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree missing %q:\n%s", want, tree)
		}
	}

	// Directories should be listed before files at the same level (root
	// has Season1/, Season2/, then readme.txt).
	season1Idx := strings.Index(tree, "Season1")
	readmeIdx := strings.Index(tree, "readme.txt")
	if season1Idx == -1 || readmeIdx == -1 || season1Idx > readmeIdx {
		t.Errorf("expected directories before files at the same level:\n%s", tree)
	}
}

func TestRenderFileTree_SingleFileNoDirectories(t *testing.T) {
	files := []engine.FileSummary{{Path: "movie.mp4", Length: 42}}
	tree := renderFileTree(files)
	if !strings.Contains(tree, "movie.mp4") {
		t.Errorf("tree missing file name:\n%s", tree)
	}
}

func TestRenderFileTree_Empty(t *testing.T) {
	tree := renderFileTree(nil)
	if tree != "" {
		t.Errorf("expected empty tree for no files, got:\n%s", tree)
	}
}
