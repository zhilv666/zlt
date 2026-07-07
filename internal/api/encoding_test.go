package api

import (
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func gbkBytes(t *testing.T, s string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("gbk encode %q: %v", s, err)
	}
	return encoded
}

func TestDecodeLogTextEmpty(t *testing.T) {
	if got := decodeLogText(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestDecodeLogTextLeavesUTF8Untouched(t *testing.T) {
	in := []byte("普通中文日志\nline two\n")
	if got := decodeLogText(in); got != string(in) {
		t.Fatalf("utf-8 content changed: %q", got)
	}
}

func TestDecodeLogTextDecodesGBK(t *testing.T) {
	in := gbkBytes(t, "微信web开发者工具")
	got := decodeLogText(in)
	if got != "微信web开发者工具" {
		t.Fatalf("expected decoded gbk, got %q", got)
	}
}

// A file that mixes a UTF-8 line and a GBK line must keep the UTF-8 line intact
// while still decoding the GBK line. This is the case the whole-buffer decoder
// got wrong (one bad line forced the entire file through GBK).
func TestDecodeLogTextMixedEncodingPerLine(t *testing.T) {
	utf8Line := "已经是UTF-8的一行"
	gbkLine := gbkBytes(t, "这一行是GBK")

	in := append([]byte(utf8Line+"\n"), gbkLine...)
	got := decodeLogText(in)

	want := utf8Line + "\n" + "这一行是GBK"
	if got != want {
		t.Fatalf("mixed decode mismatch:\n got %q\nwant %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid utf-8: %q", got)
	}
}
