package service

import (
	"strings"
	"testing"
)

func TestRewriteMarkdownImageRefs(t *testing.T) {
	assets := map[string]documentImportAsset{
		"images/a.png": {OriginalPath: "images/a.png", URL: "https://cdn.example.com/a.png"},
		"b.jpg":        {OriginalPath: "b.jpg", URL: "https://cdn.example.com/b.jpg"},
	}

	input := `# Doc

![A](images/a.png)
![Remote](https://example.com/r.png)
<img src="b.jpg" alt="B">
`

	got, count := rewriteMarkdownImageRefs(input, assets)
	if count != 2 {
		t.Fatalf("rewrite count = %d, want 2", count)
	}
	if want := "![A](https://cdn.example.com/a.png)"; !strings.Contains(got, want) {
		t.Fatalf("rewritten markdown missing %q in:\n%s", want, got)
	}
	if want := `<img src="https://cdn.example.com/b.jpg"`; !strings.Contains(got, want) {
		t.Fatalf("rewritten html image missing %q in:\n%s", want, got)
	}
	if want := "![Remote](https://example.com/r.png)"; !strings.Contains(got, want) {
		t.Fatalf("remote image should be unchanged, missing %q in:\n%s", want, got)
	}
}

func TestNormalizeAssetPathRejectsTraversal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "normalizes slashes", in: `.\images\a.png`, want: "images/a.png"},
		{name: "rejects parent", in: "../secret.png", want: ""},
		{name: "keeps basename", in: "a.png", want: "a.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAssetPath(tt.in); got != tt.want {
				t.Fatalf("normalizeAssetPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
