package repo

import "testing"

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
