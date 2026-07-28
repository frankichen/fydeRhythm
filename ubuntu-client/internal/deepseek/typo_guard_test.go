package deepseek

import "testing"

func TestEnforceTypoOnlyAcceptsSmallHanSubstitutions(t *testing.T) {
	tests := []struct {
		original  string
		candidate string
		want      string
	}{
		{"我明天在去公司", "我明天再去公司", "我明天再去公司"},
		{"这个目录不存再", "这个目录不存在", "这个目录不存在"},
		{"你好，世界！", "你好,世界!", "你好,世界!"},
	}
	for _, tt := range tests {
		if got := enforceTypoOnly(tt.original, tt.candidate); got != tt.want {
			t.Fatalf("enforceTypoOnly(%q, %q) = %q, want %q", tt.original, tt.candidate, got, tt.want)
		}
	}
}

func TestEnforceTypoOnlyRejectsRewriting(t *testing.T) {
	tests := []struct {
		name      string
		original  string
		candidate string
	}{
		{"insertion", "今天去公司", "今天准备去公司"},
		{"latin change", "发布 v1 版本", "发布 v2 版本"},
		{"too many replacements", "我今天去公司开会", "今日前往单位办公"},
		{"punctuation change", "你好,世界!", "你好;世界!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		want := normalizeEnglishPunctuation(tt.original)
		if got := enforceTypoOnly(tt.original, tt.candidate); got != want {
			t.Fatalf("got %q, want original %q", got, want)
		}
		})
	}
}
