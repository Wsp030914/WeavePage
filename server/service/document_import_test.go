package service

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import (
	"strings"
	"testing"
)

// TestRewriteMarkdownImageRefs 验证本地图片引用会被改写，而远端链接保持不变。
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

// TestNormalizeAssetPathRejectsTraversal 验证资源路径规范化会拒绝目录穿越。
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
