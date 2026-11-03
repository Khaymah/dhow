package migrations

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type SchemaState string

const (
	SchemaCurrent      SchemaState = "current"
	SchemaPending      SchemaState = "pending"
	SchemaAhead        SchemaState = "ahead"
	SchemaIncompatible SchemaState = "incompatible"
)

const (
	ExitSchemaTooNew = 78
	CodeSchemaTooNew = "SCHEMA_TOO_NEW"
	DefaultFloor     = "0.0.0"
	UpgradesDocURL   = "https://github.com/Khaymah/dhow/blob/main/docs/install/upgrades.md"
)

func (s SchemaState) Ready() bool { return s == SchemaCurrent || s == SchemaAhead }

type Binary struct {
	Version string
	Dev     bool
}

type Status struct {
	State     SchemaState
	DBVersion int64
	Latest    int64
	Floor     string
	Binary    string
	Reason    string
}

type IncompatibleError struct{ Status Status }

func (e *IncompatibleError) Error() string {
	return fmt.Sprintf("%s: %s. The database is at migration %d and this binary knows migrations up to %d. "+
		"Install dhow %s or newer, or restore the database backup taken before the upgrade. See %s",
		CodeSchemaTooNew, e.Status.Reason, e.Status.DBVersion, e.Status.Latest, e.Status.Floor, UpgradesDocURL)
}

func (e *IncompatibleError) ExitCode() int { return ExitSchemaTooNew }

var ErrSchemaTooNew = errors.New(CodeSchemaTooNew)

func (e *IncompatibleError) Is(target error) bool { return target == ErrSchemaTooNew }

func Evaluate(dbVersion, latest int64, floor string, bin Binary) Status {
	st := Status{DBVersion: dbVersion, Latest: latest, Floor: floor, Binary: bin.Version}
	switch {
	case dbVersion == latest:
		st.State = SchemaCurrent
		return st
	case dbVersion < latest:
		st.State = SchemaPending
		return st
	}
	floorVer, floorOK := parseSemver(floor)
	binVer, binOK := parseSemver(bin.Version)
	switch {
	case !floorOK:
		st.State, st.Reason = SchemaIncompatible, fmt.Sprintf("the schema floor %q in the database is not a version", floor)
	case !binOK && bin.Dev:
		st.State = SchemaAhead
	case !binOK:
		st.State, st.Reason = SchemaIncompatible, fmt.Sprintf("this is a development build (%s) on a newer database; set DHOW_DEV=true to run it anyway", bin.Version)
	case compareSemver(binVer, floorVer) < 0:
		st.State, st.Reason = SchemaIncompatible, fmt.Sprintf("the database needs dhow %s or newer, and this binary is %s", floor, bin.Version)
	default:
		st.State = SchemaAhead
	}
	return st
}

var reVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

type semver struct {
	core [3]int64
	pre  []string
}

func parseSemver(v string) (semver, bool) {
	m := reVersion.FindStringSubmatch(v)
	if m == nil {
		return semver{}, false
	}
	var out semver
	for i := range 3 {
		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil {
			return semver{}, false
		}
		out.core[i] = n
	}
	if m[4] != "" {
		out.pre = strings.Split(m[4], ".")
	}
	return out, true
}

func compareSemver(a, b semver) int {
	for i := range 3 {
		if a.core[i] != b.core[i] {
			return sign(a.core[i] - b.core[i])
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePre(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return sign(int64(len(a.pre) - len(b.pre)))
}

func comparePre(a, b string) int {
	an, aErr := strconv.ParseInt(a, 10, 64)
	bn, bErr := strconv.ParseInt(b, 10, 64)
	switch {
	case aErr == nil && bErr == nil:
		return sign(an - bn)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func sign(n int64) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
