package installer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

type keyringDB struct {
	fakeDB
	keks []StoredKek
}

func (d keyringDB) StoredKeks(context.Context, string) ([]StoredKek, error) { return d.keks, d.err }

func installedKeyring(t *testing.T, f *fakeSys) *keys.Keyring {
	t.Helper()
	ring, err := keys.ParseKeyring([]byte(f.content(KeyringPath)))
	require.NoError(t, err)
	return ring
}

func storedFor(t *testing.T, ring *keys.Keyring, state string, activated, exported *time.Time) StoredKek {
	t.Helper()
	provider, err := keys.NewFileProvider(ring)
	require.NoError(t, err)
	v := ring.Latest()
	check, err := provider.CheckValue(context.Background(), v)
	require.NoError(t, err)
	return StoredKek{Version: v, State: state, CheckValue: check, ActivatedAt: activated, LastExportedAt: exported}
}

func TestInstallWritesAPrivateKeyring(t *testing.T) {
	f, _ := installedHost(t)
	info, err := f.Stat(KeyringPath)
	require.NoError(t, err)
	assert.Equal(t, uint32(0o600), uint32(info.Mode.Perm()))
	assert.Equal(t, 0, info.UID)
	ring := installedKeyring(t, f)
	assert.Equal(t, []int32{1}, ring.Versions())
}

func TestDoctorChecksTheKeyringButNeverMakesOne(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		f, _ := installedHost(t)
		require.NoError(t, f.Remove(KeyringPath))
		app, out := newApp(f, "")
		require.ErrorIs(t, app.Doctor(context.Background(), DoctorOptions{Fix: true}), ErrUnhealthy)
		assert.Contains(t, out.String(), "The keyring is present and fits the database")
		assert.Contains(t, out.String(), "doctor never makes a new one")
		_, statErr := f.Stat(KeyringPath)
		assert.Error(t, statErr, "doctor --fix never generates a keyring")
	})
	t.Run("open to other users", func(t *testing.T) {
		f, _ := installedHost(t)
		f.files[KeyringPath].mode = FileMode{Perm: 0o644}
		app, out := newApp(f, "")
		require.ErrorIs(t, app.Doctor(context.Background(), DoctorOptions{}), ErrUnhealthy)
		assert.Contains(t, out.String(), KeyringPath+" 0644")
		app, out = newApp(f, "")
		require.NoError(t, app.Doctor(context.Background(), DoctorOptions{Fix: true}), out.String())
		assert.Equal(t, uint32(0o600), uint32(f.files[KeyringPath].mode.Perm))
	})
	t.Run("malformed", func(t *testing.T) {
		f, _ := installedHost(t)
		original := f.content(KeyringPath)
		f.put(KeyringPath, original+"kek 2 short\n", 0o600)
		app, out := newApp(f, "")
		require.ErrorIs(t, app.Doctor(context.Background(), DoctorOptions{Fix: true}), ErrUnhealthy)
		assert.Contains(t, out.String(), "KEYRING_MALFORMED")
		assert.Contains(t, out.String(), "line 2")
		assert.NotContains(t, out.String(), strings.Fields(original)[2])
		assert.Equal(t, original+"kek 2 short\n", f.content(KeyringPath))
	})
}

func TestDoctorComparesTheKeyringWithTheDatabase(t *testing.T) {
	now := time.Now()
	old := now.Add(-400 * 24 * time.Hour)
	tests := []struct {
		name  string
		keks  func(ring *keys.Keyring) []StoredKek
		wants []string
		clean bool
	}{
		{"fits and exported", func(ring *keys.Keyring) []StoredKek {
			return []StoredKek{storedFor(t, ring, keys.KekActive, &now, &now)}
		}, []string{"The keyring is present and fits the database"}, true},
		{"never exported", func(ring *keys.Keyring) []StoredKek {
			return []StoredKek{storedFor(t, ring, keys.KekActive, &now, nil)}
		}, []string{"The keyring was never exported", "keys export --recipient", "Nothing blocks Dhow"}, true},
		{"old", func(ring *keys.Keyring) []StoredKek {
			return []StoredKek{storedFor(t, ring, keys.KekActive, &old, &now)}
		}, []string{"KEK version 1 has been active for more than a year", "Nothing blocks Dhow"}, true},
		{"another keyring", func(ring *keys.Keyring) []StoredKek {
			k := storedFor(t, ring, keys.KekActive, &now, &now)
			k.CheckValue = make([]byte, keys.CheckBytes)
			return []StoredKek{k}
		}, []string{"KEYRING_MISMATCH: KEK version 1"}, false},
		{"a version is missing", func(ring *keys.Keyring) []StoredKek {
			return []StoredKek{storedFor(t, ring, keys.KekRetiring, &now, &now), {Version: 2, State: keys.KekActive, CheckValue: make([]byte, keys.CheckBytes), ActivatedAt: &now}}
		}, []string{"KEYRING_MISSING_VERSION: KEK version 2"}, false},
		{"a retired version may be gone", func(ring *keys.Keyring) []StoredKek {
			return []StoredKek{{Version: 7, State: keys.KekRetired}, storedFor(t, ring, keys.KekActive, &now, &now)}
		}, []string{"The keyring is present and fits the database"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := installedHost(t)
			app, out := newApp(f, "")
			app.DB = keyringDB{keks: tt.keks(installedKeyring(t, f))}
			err := app.Doctor(context.Background(), DoctorOptions{})
			for _, w := range tt.wants {
				assert.Contains(t, out.String(), w)
			}
			if tt.clean {
				require.NoError(t, err, out.String())
			} else {
				require.ErrorIs(t, err, ErrUnhealthy)
			}
			assert.NotContains(t, out.String(), strings.Fields(f.content(KeyringPath))[2])
		})
	}
}

func TestDoctorChecksTheComposeKeyringOwner(t *testing.T) {
	f, _ := installedHost(t)
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	require.NoError(t, f.WriteFile(ComposeKeyring, ring.Marshal(), modeContainer))
	d := doctorFor(f, Answers{Mode: ModeCompose})
	findings := d.keyring(context.Background())
	require.Len(t, findings, 1)
	assert.Equal(t, SeverityOK, findings[0].Severity)

	require.NoError(t, f.WriteFile(ComposeKeyring, ring.Marshal(), modeRootPrivate))
	findings = d.keyring(context.Background())
	require.Len(t, findings, 1)
	assert.Equal(t, SeverityFail, findings[0].Severity)
	assert.Contains(t, findings[0].Detail, "uid=0")
	require.NotNil(t, findings[0].Fix)
	assert.Contains(t, findings[0].Fix.Commands[0], "chown 65532:65532")
}

func TestTheKeyringMessagesReadInArabicToo(t *testing.T) {
	m := NewMessages(LangAr)
	assert.Contains(t, m.T(KeyCheckKeyringOld, 3), "3")
	assert.Contains(t, m.T(KeyAdviceKeyring), "--discard-unreadable")
}
