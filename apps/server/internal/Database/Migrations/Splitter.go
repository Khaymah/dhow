package migrations

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type ParsedFile struct {
	UseTx bool
	Phase string
	Floor string
	Up    []string
}

type splitState int

const (
	splitStart splitState = iota
	splitUp
	splitBlock
)

const maxLineBytes = 4 << 20

var (
	ErrSplit = errors.New("parse migration")

	rePhaseDirective = regexp.MustCompile(`(?i)^--\s*dhow:phase\s+(\S+)\s*$`)
	reFloorDirective = regexp.MustCompile(`(?i)^--\s*dhow:floor\s+(\S+)\s*$`)
	reConcurrentIdx  = regexp.MustCompile(`(?is)^CREATE\s+(?:UNIQUE\s+)?INDEX\s+CONCURRENTLY\s+(?:IF\s+NOT\s+EXISTS\s+)?("(?:[^"]|"")+"|[A-Za-z_][A-Za-z0-9_$]*)`)
)

type annotation string

const (
	annUp       annotation = "up"
	annDown     annotation = "down"
	annBegin    annotation = "statementbegin"
	annEnd      annotation = "statementend"
	annNoTx     annotation = "no transaction"
	annEnvsubOn annotation = "envsub on"
	annEnvsubNo annotation = "envsub off"
)

func splitErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSplit, fmt.Sprintf(format, args...))
}

func parseAnnotation(line string) (annotation, error) {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return "", splitErr("annotation %q has leading whitespace", line)
	}
	cmd := strings.Replace(strings.ReplaceAll(line, "--", ""), "+goose", "", 1)
	if strings.Contains(cmd, "+goose") {
		return "", splitErr("%q has more than one +goose annotation", line)
	}
	a := annotation(strings.ToLower(strings.TrimSpace(cmd)))
	switch a {
	case annUp, annDown, annBegin, annEnd, annNoTx:
		return a, nil
	case annEnvsubOn, annEnvsubNo:
		return "", splitErr("%q: environment substitution is not used in dhow migrations", line)
	}
	return "", splitErr("unknown annotation %q", line)
}

func endsStatement(line string) bool {
	prev := ""
	for _, word := range strings.Fields(line) {
		if strings.HasPrefix(word, "--") {
			break
		}
		prev = word
	}
	return strings.HasSuffix(prev, ";")
}

type splitter struct {
	out   ParsedFile
	state splitState
	buf   strings.Builder
	done  bool
}

func (s *splitter) emit() {
	s.out.Up = append(s.out.Up, strings.TrimSpace(s.buf.String()))
	s.buf.Reset()
}

func (s *splitter) annotate(line string) error {
	a, err := parseAnnotation(line)
	if err != nil {
		return err
	}
	switch {
	case a == annNoTx:
		s.out.UseTx = false
	case a == annUp && s.state == splitStart:
		s.state = splitUp
	case a == annUp:
		return splitErr("duplicate -- +goose Up")
	case a == annDown && s.state == splitUp:
		if rest := strings.TrimSpace(s.buf.String()); rest != "" {
			return splitErr("unfinished statement before -- +goose Down: %q (missing semicolon?)", rest)
		}
		s.done = true
	case a == annDown:
		return splitErr("-- +goose Down must follow -- +goose Up and close every StatementBegin")
	case a == annBegin && s.state == splitUp:
		s.state = splitBlock
	case a == annBegin:
		return splitErr("-- +goose StatementBegin must follow -- +goose Up")
	case a == annEnd && s.state == splitBlock:
		s.emit()
		s.state = splitUp
	default:
		return splitErr("-- +goose StatementEnd must follow -- +goose StatementBegin")
	}
	return nil
}

func (s *splitter) header(trimmed string) error {
	if m := rePhaseDirective.FindStringSubmatch(trimmed); m != nil {
		s.out.Phase = strings.ToLower(m[1])
		return nil
	}
	if m := reFloorDirective.FindStringSubmatch(trimmed); m != nil {
		s.out.Floor = m[1]
		return nil
	}
	if strings.HasPrefix(trimmed, "--") {
		return nil
	}
	return splitErr("statement %q comes before -- +goose Up", trimmed)
}

func (s *splitter) line(line string) error {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "--") && strings.Contains(line, "+goose") {
		return s.annotate(line)
	}
	if s.state == splitStart {
		if trimmed == "" {
			return nil
		}
		return s.header(trimmed)
	}
	if s.buf.Len() == 0 && (strings.HasPrefix(trimmed, "--") || line == "") {
		return nil
	}
	s.buf.WriteString(line)
	s.buf.WriteByte('\n')
	if s.state == splitUp && endsStatement(line) {
		s.emit()
	}
	return nil
}

func ParseUp(text string) (ParsedFile, error) {
	s := &splitter{out: ParsedFile{UseTx: true}}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for !s.done && scanner.Scan() {
		if err := s.line(scanner.Text()); err != nil {
			return ParsedFile{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedFile{}, splitErr("read: %v", err)
	}
	switch s.state {
	case splitStart:
		return ParsedFile{}, splitErr("missing -- +goose Up")
	case splitBlock:
		return ParsedFile{}, splitErr("missing -- +goose StatementEnd")
	case splitUp:
	}
	if rest := strings.TrimSpace(s.buf.String()); rest != "" {
		return ParsedFile{}, splitErr("unfinished statement %q (missing semicolon?)", rest)
	}
	return s.out, nil
}

func ConcurrentIndexNames(stmts []string) []string {
	var names []string
	for _, stmt := range stmts {
		m := reConcurrentIdx.FindStringSubmatch(strings.TrimSpace(stmt))
		if m == nil {
			continue
		}
		name := m[1]
		if strings.HasPrefix(name, `"`) {
			name = strings.ReplaceAll(name[1:len(name)-1], `""`, `"`)
		} else {
			name = strings.ToLower(name)
		}
		names = append(names, name)
	}
	return names
}
