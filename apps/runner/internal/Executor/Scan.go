package executor

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
)

const scanBuffer = 32 << 10

func (a *attempt) secretIn(path string) (string, bool, error) {
	finder := a.scan.Finder()
	if finder.Empty() {
		return "", false, nil
	}
	f, err := os.Open(path) //nolint:gosec // The packed file in the attempt directory of this runner.
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	buf := make([]byte, scanBuffer)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		finder.Reset()
		if finder.Found([]byte(hdr.Name)) || finder.Found([]byte(hdr.Linkname)) {
			return hdr.Name, true, nil
		}
		finder.Reset()
		hit, err := found(finder, tr, buf)
		if err != nil {
			return "", false, err
		}
		if hit {
			return hdr.Name, true, nil
		}
	}
}

type matcher interface{ Found(p []byte) bool }

func found(f matcher, r io.Reader, buf []byte) (bool, error) {
	for {
		n, err := r.Read(buf)
		if n > 0 && f.Found(buf[:n]) {
			return true, nil
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
