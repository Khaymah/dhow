package stack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const maxRecordBytes = 64 << 10

type record struct {
	ComposeSum  string   `json:"ComposeSum"`
	OverrideSum string   `json:"OverrideSum"`
	Links       []string `json:"Links"`
	AllowUnsafe bool     `json:"AllowUnsafe"`
}

func sum(raw []byte) string {
	s := sha256.Sum256(raw)
	return hex.EncodeToString(s[:])
}

func newRecord(compose, override []byte, links []string, allowUnsafe bool) record {
	return record{ComposeSum: sum(compose), OverrideSum: sum(override), Links: links, AllowUnsafe: allowUnsafe}
}

func recordPath(rel deployrelease.Release) string {
	return rel.DeployPath + "/.dhow/compose-" + rel.ID + ".json"
}

func writeRecord(r deployrelease.Remote, rel deployrelease.Release, rec record) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return deployrelease.WriteFile(r, recordPath(rel), raw, 0o644)
}

func readRecord(r deployrelease.Remote, rel deployrelease.Release) (record, error) {
	raw, err := r.ReadFile(recordPath(rel), maxRecordBytes+1)
	if err != nil || len(raw) > maxRecordBytes {
		return record{}, changed("the record of the checked compose files is missing")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var rec record
	if err := dec.Decode(&rec); err != nil {
		return record{}, changed(fmt.Sprintf("the record of the checked compose files is not valid: %v", err))
	}
	return rec, nil
}
