package process

import (
	"bytes"
	"encoding/binary"
	"strconv"
)

// KERN_PROCARGS2 contains a native int32 argc, an executable path, padding,
// exactly argc NUL-terminated argv entries, then environment entries.
func parseDarwinProcArgs(data []byte) (string, []string, bool) {
	if len(data) < 4 {
		return "", nil, false
	}
	argc := int(binary.LittleEndian.Uint32(data[:4])) // Both supported Mac architectures are little-endian.
	data = data[4:]
	end := bytes.IndexByte(data, 0)
	if argc <= 0 || argc > len(data) || end <= 0 {
		return "", nil, false
	}
	exe := string(data[:end])
	data = data[end+1:]
	for len(data) > 0 && data[0] == 0 {
		data = data[1:]
	}
	argv := make([]string, 0, argc)
	for range argc {
		end = bytes.IndexByte(data, 0)
		if end < 0 {
			return "", nil, false
		}
		argv = append(argv, string(data[:end]))
		data = data[end+1:]
	}
	return exe, argv, true
}

func parseDarwinCWDIdentity(data []byte) (uint64, uint64, bool) {
	inCWD := false
	var device, inode uint64
	var hasDevice, hasInode bool
	for _, field := range bytes.Split(data, []byte{0}) {
		// lsof separates process/file record sets with a newline even in -F0.
		field = bytes.TrimLeft(field, "\n")
		if len(field) == 0 {
			continue
		}
		switch field[0] {
		case 'p':
			inCWD = false
			hasDevice, hasInode = false, false
		case 'f':
			inCWD = string(field[1:]) == "cwd"
			hasDevice, hasInode = false, false
		case 'D':
			if inCWD {
				var err error
				device, err = strconv.ParseUint(string(field[1:]), 0, 64)
				hasDevice = err == nil
			}
		case 'i':
			if inCWD {
				var err error
				inode, err = strconv.ParseUint(string(field[1:]), 10, 64)
				hasInode = err == nil
			}
		}
		if hasDevice && hasInode {
			return device, inode, true
		}
	}
	return 0, 0, false
}
