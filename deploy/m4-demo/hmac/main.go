package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

const usage = "usage: hmac KEY_FILE < BODY\nprints the hex HMAC-SHA256 of stdin, keyed with the content of KEY_FILE without its trailing newline\n"

func run(args []string, in io.Reader, out io.Writer) int {
	if len(args) != 1 {
		_, _ = os.Stderr.WriteString(usage)
		return 2
	}
	key, err := os.ReadFile(args[0]) //nolint:gosec // The demo passes the path of its own key file.
	if err != nil {
		_, _ = os.Stderr.WriteString("cannot read the key file\n")
		return 1
	}
	key = bytes.TrimRight(key, "\n")
	if len(key) == 0 {
		_, _ = os.Stderr.WriteString("the key file is empty\n")
		return 1
	}
	mac := hmac.New(sha256.New, key)
	if _, err := io.Copy(mac, in); err != nil {
		_, _ = os.Stderr.WriteString("cannot read the body\n")
		return 1
	}
	if _, err := io.WriteString(out, hex.EncodeToString(mac.Sum(nil))+"\n"); err != nil {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout)) }
