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

func TestCommandMatchesTaskWithArgs(t *testing.T) {
	cases := []struct {
		name        string
		commandLine string
		programPath string
		args        []string
		want        bool
	}{
		{
			name:        "matches python main script",
			commandLine: `/usr/bin/python3 main.py`,
			programPath: `/usr/bin/python3`,
			args:        []string{"main.py"},
			want:        true,
		},
		{
			name:        "does not match different script",
			commandLine: `/usr/bin/python3 other.py`,
			programPath: `/usr/bin/python3`,
			args:        []string{"main.py"},
			want:        false,
		},
		{
			name:        "does not match missing args",
			commandLine: `/usr/bin/python3`,
			programPath: `/usr/bin/python3`,
			args:        []string{"main.py"},
			want:        false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandMatchesTask(tc.commandLine, tc.programPath, tc.args); got != tc.want {
				t.Fatalf("unexpected task match result: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSameWorkingDir(t *testing.T) {
	cases := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{
			name:  "matches identical unix path",
			left:  "/home/ubuntu/code/cksd",
			right: "/home/ubuntu/code/cksd",
			want:  true,
		},
		{
			name:  "matches normalized windows separators",
			left:  `C:\work\demo`,
			right: `C:/work/demo`,
			want:  true,
		},
		{
			name:  "different working directory",
			left:  "/home/ubuntu/code/cksd",
			right: "/home/ubuntu/code/other",
			want:  false,
		},
		{
			name:  "empty path does not match",
			left:  "",
			right: "/home/ubuntu/code/cksd",
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameWorkingDir(tc.left, tc.right); got != tc.want {
				t.Fatalf("unexpected workdir match result: got %v want %v", got, tc.want)
			}
		})
	}
}
