package deployrelease

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

var stateNames = []string{"idle", "switching", "rolling_back"}

type dhowFacts struct {
	hasState bool
	state    stateFile
	verified string
}

func corrupt(file string) error { return fmt.Errorf("%w: %s", ErrStateCorrupt, file) }

func readSmallFrom(r Remote, name string) ([]byte, bool, error) {
	raw, err := r.ReadFile(name, maxSmallFile)
	switch {
	case err == nil:
		return raw, true, nil
	case isNotExist(err):
		return nil, false, nil
	default:
		return nil, false, err
	}
}

func validState(raw []byte) (stateFile, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s stateFile
	ok := dec.Decode(&s) == nil && !dec.More() && slices.Contains(stateNames, s.State) && validID(s.Release) &&
		(s.Previous == "" || validID(s.Previous))
	return s, ok
}

func readDhow(r Remote, dhow string) (dhowFacts, error) {
	var out dhowFacts
	raw, found, err := readSmallFrom(r, dhow+"/"+stateName)
	if err != nil {
		return out, corrupt(stateName)
	}
	if found {
		s, ok := validState(raw)
		if !ok {
			return out, corrupt(stateName)
		}
		out.hasState, out.state = true, s
	}
	raw, found, err = readSmallFrom(r, dhow+"/"+verifiedName)
	if err != nil {
		return out, corrupt(verifiedName)
	}
	if found {
		v := strings.TrimSpace(string(raw))
		if !validID(v) {
			return out, corrupt(verifiedName)
		}
		out.verified = v
	}
	return out, checkHistory(r, dhow)
}

func checkHistory(r Remote, dhow string) error {
	raw, err := r.ReadFile(dhow+"/"+historyName, maxHistoryBytes)
	if isNotExist(err) {
		return nil
	}
	if err != nil {
		return corrupt(historyName)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if v := strings.TrimSpace(line); v != "" && !validID(v) {
			return corrupt(historyName)
		}
	}
	return nil
}

func stateFailure(err error) *Failure {
	return fail(ExitStateCorrupt, CauseOther, "a state file of Dhow on the server is damaged", err)
}
