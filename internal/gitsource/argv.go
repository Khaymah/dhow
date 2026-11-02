package gitsource

import (
	"errors"
	"strings"

	"github.com/Khaymah/dhow/internal/gitref"
)

const (
	MaxDepth      = 1 << 16
	MaxPathBytes  = 4096
	objectIDBytes = 64
)

var (
	ErrDepth      = errors.New("the fetch depth must be from 1 to 65536")
	ErrFilePath   = errors.New("the file path is empty, absolute, too long, or has a control character or a parent directory")
	ErrObjectID   = errors.New("an object id is 40 or 64 lowercase hexadecimal characters")
	ErrNotFetched = errors.New("only https and ssh repositories are fetched")
)

func ListFileArgs(sha, path string) ([]string, error) {
	if err := gitref.CheckSha(sha); err != nil {
		return nil, err
	}
	if err := CheckFilePath(path); err != nil {
		return nil, err
	}
	return append(SafeConfig(), "ls-tree", "-z", "--full-tree", sha, "--", path), nil
}

func ReadBlobArgs(objectID string) ([]string, error) {
	if !gitref.ValidSha(objectID) || len(objectID) > objectIDBytes {
		return nil, ErrObjectID
	}
	return append(SafeConfig(), "cat-file", "blob", objectID), nil
}

func CheckFilePath(path string) error {
	if path == "" || len(path) > MaxPathBytes || strings.HasPrefix(path, "/") {
		return ErrFilePath
	}
	for i := 0; i < len(path); i++ {
		if path[i] < ' ' || path[i] == 0x7f {
			return ErrFilePath
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return ErrFilePath
		}
	}
	return nil
}

func BlobFromListing(listing []byte, path string) (string, bool) {
	entry, _, _ := strings.Cut(string(listing), "\x00")
	meta, name, ok := strings.Cut(entry, "\t")
	if !ok || name != path {
		return "", false
	}
	fields := strings.Fields(meta)
	if len(fields) != 3 || fields[1] != "blob" || !gitref.ValidSha(fields[2]) {
		return "", false
	}
	return fields[2], true
}
