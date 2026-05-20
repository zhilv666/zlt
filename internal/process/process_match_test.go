package process

import "testing"

func TestExtractCommandPath(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantOut string
	}{
		{name: "quoted path", line: `"C:\Program Files\OpenList\openlist.exe" server`, wantOut: `C:\Program Files\OpenList\openlist.exe`},
		{name: "plain path", line: `/usr/local/bin/openlist server`, wantOut: `/usr/local/bin/openlist`},
		{name: "empty", line: `   `, wantOut: ``},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractCommandPath(tc.line); got != tc.wantOut {
				t.Fatalf("unexpected command path: got %q want %q", got, tc.wantOut)
			}
		})
	}
}

func TestCommandMatchesProgram(t *testing.T) {
	cases := []struct {
		name        string
		commandLine string
		programPath string
		want        bool
	}{
		{
			name:        "matches quoted windows path",
			commandLine: `"C:\Program Files\OpenList\openlist.exe" server`,
			programPath: `C:\Program Files\OpenList\openlist.exe`,
			want:        true,
		},
		{
			name:        "matches unix path",
			commandLine: `/usr/local/bin/openlist server --port 5244`,
			programPath: `/usr/local/bin/openlist`,
			want:        true,
		},
		{
			name:        "matches basename",
			commandLine: `openlist.exe server`,
			programPath: `D:\SoftWare\OpenList\openlist.exe`,
			want:        true,
		},
		{
			name:        "different program",
			commandLine: `/usr/bin/python app.py`,
			programPath: `/usr/local/bin/openlist`,
			want:        false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandMatchesProgram(tc.commandLine, tc.programPath); got != tc.want {
				t.Fatalf("unexpected match result: got %v want %v", got, tc.want)
			}
		})
	}
}
