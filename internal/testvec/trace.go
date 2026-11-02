package testvec

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"strings"
)

// ReadTrace returns the lines of a (possibly gzip-compressed) trace file.
func ReadTrace(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// WriteTrace writes lines to path, gzip-compressed when path ends in .gz.
func WriteTrace(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var w io.Writer = f
	var gz *gzip.Writer
	if strings.HasSuffix(path, ".gz") {
		gz, _ = gzip.NewWriterLevel(f, gzip.BestCompression)
		w = gz
	}
	bw := bufio.NewWriter(w)
	for _, l := range lines {
		if _, err := bw.WriteString(l); err != nil {
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if gz != nil {
		return gz.Close()
	}
	return nil
}

// FirstDiff returns the index of the first differing line, or -1 if equal.
func FirstDiff(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}
