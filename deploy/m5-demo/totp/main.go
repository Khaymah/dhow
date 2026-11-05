package main

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP with SHA-1 is what the server and every authenticator app use.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	usage   = "usage: totp STATE_FILE < SECRET\nprints the next unused 6-digit code for the base32 secret on stdin and records its step in STATE_FILE\n"
	period  = 30
	digits  = 6
	skew    = 1
	noStep  = int64(-1)
	maxWait = 3 * period * time.Second
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func code(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // The step is a positive time counter.
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", digits, value%1000000)
}

func lastStep(path string) (int64, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // The demo passes the path of its own state file.
	if os.IsNotExist(err) {
		return noStep, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the state file: %w", err)
	}
	step, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("the state file does not hold a step: %w", err)
	}
	return step, nil
}

func nextStep(last int64, now time.Time) (int64, time.Duration) {
	current := now.Unix() / period
	step := max(last+1, current)
	if step <= current+skew {
		return step, 0
	}
	if step > current+skew+3 {
		return step, maxWait + time.Second
	}
	opens := time.Unix((step-skew)*period, 0)
	return step, opens.Sub(now) + time.Second
}

func run(args []string, in io.Reader, out io.Writer, now func() time.Time, sleep func(time.Duration)) int {
	if len(args) != 1 {
		_, _ = os.Stderr.WriteString(usage)
		return 2
	}
	raw, err := io.ReadAll(io.LimitReader(in, 256))
	if err != nil {
		_, _ = os.Stderr.WriteString("cannot read the secret\n")
		return 1
	}
	secret, err := encoding.DecodeString(strings.ToUpper(strings.TrimSpace(string(raw))))
	if err != nil || len(secret) == 0 {
		_, _ = os.Stderr.WriteString("the secret is not base32\n")
		return 1
	}
	last, err := lastStep(args[0])
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	step, wait := nextStep(last, now())
	if wait > maxWait {
		_, _ = os.Stderr.WriteString("the state file is too far ahead of the clock\n")
		return 1
	}
	if wait > 0 {
		sleep(wait)
	}
	if err := os.WriteFile(args[0], []byte(strconv.FormatInt(step, 10)+"\n"), 0o600); err != nil {
		_, _ = os.Stderr.WriteString("cannot write the state file\n")
		return 1
	}
	if _, err := io.WriteString(out, code(secret, step)+"\n"); err != nil {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, time.Now, time.Sleep)) }
