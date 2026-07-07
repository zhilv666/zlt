package api

import (
	"bytes"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// decodeLogText normalizes log bytes to UTF-8 for display in the web UI.
//
// On Chinese Windows, cmd.exe and most console programs emit output in GBK
// (codepage 936). The web UI serves log content as UTF-8, so those bytes would
// otherwise render as mojibake.
//
// Decoding is done line by line rather than over the whole buffer. A log file
// can mix UTF-8 and GBK output (different child programs), and the tail of a
// file that is still being written may end mid-character. Deciding per line
// keeps any damage local to the affected line instead of forcing the entire
// file through one interpretation.
func decodeLogText(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		lines[i] = []byte(decodeLogLine(line))
	}
	return string(bytes.Join(lines, []byte("\n")))
}

// decodeLogLine returns line as UTF-8. A line that is already valid UTF-8 is
// left untouched; otherwise a GBK decode is attempted and kept only when it
// produces valid UTF-8. If neither works the original bytes are returned so we
// never lose data.
func decodeLogLine(line []byte) string {
	if utf8.Valid(line) {
		return string(line)
	}

	if decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), line); err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}

	return string(line)
}
