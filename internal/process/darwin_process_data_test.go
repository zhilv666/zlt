package process

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func darwinArgsFixture(exe string, argv, env []string) []byte {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, uint32(len(argv)))
	data = append(data, exe...)
	data = append(data, 0, 0, 0)
	for _, arg := range append(append([]string{}, argv...), env...) {
		data = append(data, arg...)
		data = append(data, 0)
	}
	return data
}

func TestDarwinProcArgsPreservesBoundaries(t *testing.T) {
	exe := "/opt/service a/worker"
	for _, argv := range [][]string{
		{exe},
		{exe, "path with spaces", "", `quoted"value`, `C:\trailing\`, "line\nbreak", ""},
	} {
		data := darwinArgsFixture(exe, argv, []string{"TOKEN=not-an-argument", "EMPTY="})
		gotExe, gotArgv, ok := parseDarwinProcArgs(data)
		if !ok || gotExe != exe || !reflect.DeepEqual(gotArgv, argv) {
			t.Fatalf("got %q %q %v; want %q %q", gotExe, gotArgv, ok, exe, argv)
		}
	}
}

func TestDarwinProcArgsRejectsIncompleteData(t *testing.T) {
	valid := darwinArgsFixture("/opt/worker", []string{"worker", "argument"}, nil)
	badCount := append([]byte{}, valid...)
	binary.LittleEndian.PutUint32(badCount, ^uint32(0))
	for _, data := range [][]byte{nil, {1, 0, 0}, {0, 0, 0, 0}, badCount} {
		if _, _, ok := parseDarwinProcArgs(data); ok {
			t.Fatalf("accepted malformed process arguments: %v", data)
		}
	}
	for end := range len(valid) {
		if _, _, ok := parseDarwinProcArgs(valid[:end]); ok {
			t.Fatalf("accepted process arguments truncated at %d", end)
		}
	}
}

func TestDarwinCWDIdentityFields(t *testing.T) {
	for _, tc := range []struct {
		data          string
		device, inode uint64
		ok            bool
	}{
		{"p123\x00\nfcwd\x00D0x1000010\x00i9876\x00\n", 0x1000010, 9876, true},
		{"p123\x00\nfcwd\x00i9876\x00D0x1000010\x00\n", 0x1000010, 9876, true},
		{"p123\x00\nf1\x00D0x10\x00i1234\x00\nfcwd\x00D0x20\x00i5678\x00\n", 0x20, 5678, true},
		{"p123\x00\nf1\x00D0x10\x00i1234\x00\n", 0, 0, false},
		{"p123\x00\nfcwd\x00D0x10\x00\nf1\x00i1234\x00\n", 0, 0, false},
		{"p123\x00\nfcwd\x00Dunknown\x00i1234\x00\n", 0, 0, false},
		{"p123\x00\nfcwd\x00D0x10\x00iunknown\x00\n", 0, 0, false},
		{"", 0, 0, false},
	} {
		device, inode, ok := parseDarwinCWDIdentity([]byte(tc.data))
		if device != tc.device || inode != tc.inode || ok != tc.ok {
			t.Fatalf("for %q: got %d/%d, %v; want %d/%d, %v", tc.data, device, inode, ok, tc.device, tc.inode, tc.ok)
		}
	}
}
