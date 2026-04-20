package repo

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import "testing"

// TestNormalizeTaskContentUpdateLimit 验证正文增量同步 limit 会被规范到允许范围内。
func TestNormalizeTaskContentUpdateLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "default for zero", limit: 0, want: defaultTaskContentUpdateLimit},
		{name: "default for negative", limit: -1, want: defaultTaskContentUpdateLimit},
		{name: "keeps valid limit", limit: 20, want: 20},
		{name: "caps oversized limit", limit: maxTaskContentUpdateLimit + 1, want: maxTaskContentUpdateLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeTaskContentUpdateLimit(tt.limit)
			if got != tt.want {
				t.Fatalf("normalizeTaskContentUpdateLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}
