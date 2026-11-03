package installer

import (
	"context"
	"crypto/hmac"
	"fmt"
	"io/fs"
	"strings"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const keyringPrivateBits = 0o077

type KeyringReader interface {
	StoredKeks(ctx context.Context, url string) ([]StoredKek, error)
}

type StoredKek struct {
	Version        int32
	State          string
	CheckValue     []byte
	Unreadable     int64
	ActivatedAt    *time.Time
	LastExportedAt *time.Time
}

func (PostgresChecker) StoredKeks(ctx context.Context, url string) ([]StoredKek, error) {
	ctx, cancel := context.WithTimeout(ctx, databaseCheckTimeout)
	defer cancel()
	pool, err := config.OpenDatabase(ctx, url)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	q := sqlc.New(pool)
	rows, err := q.ListKeyEncryptionKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the KEK versions: %w", err)
	}
	counts, err := q.CountDataKeysByVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("count the data keys: %w", err)
	}
	unreadable := map[int32]int64{}
	for _, c := range counts {
		unreadable[c.KekVersion] = c.Unreadable
	}
	out := make([]StoredKek, 0, len(rows))
	for _, r := range rows {
		out = append(out, StoredKek{
			Version: r.Version, State: r.State, CheckValue: r.CheckValue, Unreadable: unreadable[r.Version],
			ActivatedAt: r.ActivatedAt, LastExportedAt: r.LastExportedAt,
		})
	}
	return out, nil
}

func (d Doctor) keyringFile() (string, int) {
	if d.A.Mode == ModeCompose {
		return ComposeKeyring, ContainerUID
	}
	return KeyringPath, 0
}

func (d Doctor) keyring(ctx context.Context) []Finding {
	path, owner := d.keyringFile()
	title := d.M.T(KeyCheckKeyring)
	failed := func(detail string) []Finding {
		return []Finding{{Severity: SeverityFail, Title: title, Detail: detail, Advice: d.M.T(KeyAdviceKeyring)}}
	}
	info, err := d.Sys.Lstat(path)
	if err != nil {
		return failed(path + ": " + err.Error())
	}
	if info.Mode&fs.ModeSymlink != 0 {
		return failed(path + " is a symbolic link; the server refuses it, so put the keyring file itself there")
	}
	if info.IsDir || info.Mode.Perm()&keyringPrivateBits != 0 || info.UID != owner {
		return []Finding{d.keyringPermissions(path, owner, info)}
	}
	data, err := d.Sys.ReadFile(path)
	if err != nil {
		return failed(path + ": " + err.Error())
	}
	defer clear(data)
	ring, err := keys.ParseKeyring(data)
	if err != nil {
		return failed(path + ": " + err.Error())
	}
	defer ring.Destroy()
	stored, known := d.storedKeks(ctx)
	if !known {
		return []Finding{ok(title)}
	}
	if problems := keyringProblems(ctx, ring, stored); len(problems) > 0 {
		return failed(strings.Join(problems, "; "))
	}
	out := []Finding{ok(title)}
	for _, s := range stored {
		if s.Unreadable > 0 {
			out = append(out, Finding{
				Severity: SeverityWarn, Title: d.M.T(KeyCheckKeyringUnreadable, s.Unreadable, s.Version), Advice: d.M.T(KeyAdviceKeyringUnreadable),
			})
		}
	}
	return append(out, d.keyringAdvisories(stored, time.Now())...)
}

func (d Doctor) keyringPermissions(path string, owner int, info FileInfo) Finding {
	want := FileMode{Perm: modeRootPrivate.Perm}
	if owner != 0 {
		want = modeContainer
	}
	fix := Step{Title: d.M.T(KeyFixPermissions), Apply: func(context.Context) error { return d.Sys.Chmod(path, want) }}
	fix.Commands = []string{fmt.Sprintf("chown %d:%d %s && chmod 0600 %s", owner, owner, path, path)}
	return Finding{
		Severity: SeverityFail, Title: d.M.T(KeyCheckPermissions),
		Detail: fmt.Sprintf("%s %04o uid=%d (%s 0600 uid=%d)", path, info.Mode.Perm(), info.UID, d.M.T(KeyWant), owner),
		Fix:    &fix,
	}
}

func (d Doctor) storedKeks(ctx context.Context) ([]StoredKek, bool) {
	reader, isReader := d.Settings.(KeyringReader)
	if !isReader || d.A.Mode == ModeCompose {
		return nil, false
	}
	url, err := d.secretContent(DatabaseURLPath)
	if err != nil {
		return nil, false
	}
	stored, err := reader.StoredKeks(ctx, url)
	return stored, err == nil
}

func keyringProblems(ctx context.Context, ring *keys.Keyring, stored []StoredKek) []string {
	provider, err := keys.NewFileProvider(ring)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, s := range stored {
		if !ring.Has(s.Version) {
			if s.State != keys.KekRetired {
				out = append(out, fmt.Sprintf("%s: KEK version %d", keys.CodeKeyringMissingVersion, s.Version))
			}
			continue
		}
		check, err := provider.CheckValue(ctx, s.Version)
		if err != nil || !hmac.Equal(check, s.CheckValue) {
			out = append(out, fmt.Sprintf("%s: KEK version %d", keys.CodeKeyringMismatch, s.Version))
		}
	}
	return out
}

func (d Doctor) keyringAdvisories(stored []StoredKek, now time.Time) []Finding {
	var out []Finding
	exported := false
	for _, s := range stored {
		exported = exported || s.LastExportedAt != nil
		if s.State == keys.KekActive && s.ActivatedAt != nil && now.Sub(*s.ActivatedAt) > config.KekActiveMaxAge {
			out = append(out, Finding{Severity: SeverityWarn, Advisory: true, Title: d.M.T(KeyCheckKeyringOld, s.Version), Advice: d.M.T(KeyAdviceKeyringOld)})
		}
	}
	if !exported {
		out = append(out, Finding{Severity: SeverityWarn, Advisory: true, Title: d.M.T(KeyCheckKeyringExport), Advice: d.M.T(KeyAdviceKeyringExport)})
	}
	return out
}
