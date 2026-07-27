package deepseek

import "testing"

func TestNormalizeEnglishPunctuation(t *testing.T) {
	input := "你好，世界！这是“测试”：可以吗？（可以）；价格￥10……"
	want := "你好,世界!这是\"测试\":可以吗?(可以);价格$10..."
	if got := normalizeEnglishPunctuation(input); got != want {
		t.Fatalf("normalizeEnglishPunctuation(%q) = %q, want %q", input, got, want)
	}
}
