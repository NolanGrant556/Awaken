package filesafe

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const binaryProbeBytes = 8192

var noisyDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	".cache":       true,
}

var binaryExtensions = map[string]bool{
	".a": true, ".app": true, ".bin": true, ".class": true, ".dylib": true,
	".exe": true, ".gif": true, ".gz": true, ".ico": true, ".jpg": true,
	".jpeg": true, ".mov": true, ".mp3": true, ".mp4": true, ".o": true,
	".pdf": true, ".png": true, ".so": true, ".tar": true, ".webp": true,
	".zip": true,
}

func IsNoisyDir(name string) bool {
	return noisyDirs[name]
}

func ShouldSkipDir(path string) bool {
	return IsNoisyDir(filepath.Base(path))
}

func HasBinaryExtension(path string) bool {
	return binaryExtensions[strings.ToLower(filepath.Ext(path))]
}

func LooksBinary(path string) (bool, error) {
	if HasBinaryExtension(path) {
		return true, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buf := make([]byte, binaryProbeBytes)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	buf = buf[:n]
	for _, b := range buf {
		if b == 0 {
			return true, nil
		}
	}
	return hasManyControlBytes(buf), nil
}

func hasManyControlBytes(buf []byte) bool {
	controlBytes := 0
	for _, b := range buf {
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			controlBytes++
		}
	}
	return controlBytes > 0 && controlBytes*100/len(buf) > 10
}
