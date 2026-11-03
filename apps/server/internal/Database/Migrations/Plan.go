package migrations

import (
	"context"
	"fmt"
	"io/fs"
	"path"

	"github.com/pressly/goose/v3"
)

type PlanEntry struct {
	Version int64
	Name    string
	Phase   string
	Floor   string
	UseTx   bool
	Applied bool
	Indexes []string
	Up      []string
}

func (m *Migrator) parse(name string) (ParsedFile, error) {
	body, err := fs.ReadFile(m.fsys, name)
	if err != nil {
		return ParsedFile{}, fmt.Errorf("read migration %s: %w", name, err)
	}
	parsed, err := ParseUp(string(body))
	if err != nil {
		return ParsedFile{}, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}

func (m *Migrator) Plan(ctx context.Context) ([]PlanEntry, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the migration status: %w", err)
	}
	out := make([]PlanEntry, 0, len(statuses))
	for _, s := range statuses {
		name := path.Base(s.Source.Path)
		parsed, err := m.parse(name)
		if err != nil {
			return nil, err
		}
		out = append(out, PlanEntry{
			Version: s.Source.Version,
			Name:    name,
			Phase:   parsed.Phase,
			Floor:   parsed.Floor,
			UseTx:   parsed.UseTx,
			Applied: s.State == goose.StateApplied,
			Indexes: ConcurrentIndexNames(parsed.Up),
			Up:      parsed.Up,
		})
	}
	return out, nil
}

func Pending(entries []PlanEntry) []PlanEntry {
	var out []PlanEntry
	for _, e := range entries {
		if !e.Applied {
			out = append(out, e)
		}
	}
	return out
}
