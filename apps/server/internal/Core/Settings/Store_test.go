package instancesettings

import (
	"crypto/rand"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	"github.com/Khaymah/dhow/internal/logging"
)

func newStore(t *testing.T, env settingspec.Raw) *Store {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	s, err := New(Options{Values: keys.NewValues(keys.NewEnvelope(nil, nil), key), Env: env, Log: logging.New(io.Discard, slog.LevelError)})
	require.NoError(t, err)
	return s
}

func TestEachSettingResolvesFromTheEnvironmentThenTheDatabaseThenTheDefault(t *testing.T) {
	tests := []struct {
		name       string
		env        settingspec.Raw
		stored     settingspec.Raw
		wantSource Source
		wantValue  int
		wantLocked bool
	}{
		{"nothing set takes the default", nil, nil, SourceDefault, 300, false},
		{"a saved value wins over the default", nil, settingspec.Raw{settingspec.EnvRateLimitPublic: "40"}, SourceDatabase, 40, false},
		{"the environment wins over the default", settingspec.Raw{settingspec.EnvRateLimitPublic: "50"}, nil, SourceEnv, 50, true},
		{"the environment wins over a saved value", settingspec.Raw{settingspec.EnvRateLimitPublic: "50"}, settingspec.Raw{settingspec.EnvRateLimitPublic: "40"}, SourceEnv, 50, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore(t, tt.env)
			stored := map[settingspec.Group]Stored{}
			if tt.stored != nil {
				stored[settingspec.GroupRateLimits] = Stored{Raw: tt.stored, Version: 1}
			}
			snap, err := s.build(stored, s.emptyOidc())
			require.NoError(t, err)
			assert.Equal(t, tt.wantSource, snap.Sources[settingspec.EnvRateLimitPublic])
			assert.Equal(t, tt.wantValue, snap.Values.RateLimits.Public)
			assert.Equal(t, tt.wantLocked, snap.Locked(settingspec.EnvRateLimitPublic))
			assert.Equal(t, SourceDefault, snap.Sources[settingspec.EnvRateLimitInvitesUser], "every key resolves on its own")
		})
	}
}

func TestASecretIsSetFromEitherSourceAndNeverShownAsText(t *testing.T) {
	s := newStore(t, nil)
	s.env = settingspec.Raw{settingspec.EnvSMTPPassword: "from-env"}
	snap, err := s.build(map[settingspec.Group]Stored{settingspec.GroupMail: {Raw: settingspec.Raw{
		settingspec.EnvSMTPHost: "smtp.example.com", settingspec.EnvSMTPFrom: "dhow@example.com", settingspec.EnvSMTPUsername: "dhow",
	}}}, s.emptyOidc())
	require.NoError(t, err)
	assert.True(t, snap.SecretSet(settingspec.EnvSMTPPassword))
	assert.True(t, snap.Locked(settingspec.EnvSMTPPassword))
	assert.Empty(t, snap.Text(settingspec.EnvSMTPPassword), "Text never returns a secret")
	assert.Equal(t, "from-env", snap.Values.Mail.Password)
}

func TestTheSealedSecretsRoundTripOnlyForTheirGroup(t *testing.T) {
	s := newDBStore(t, dbtest.NewPool(t), nil)
	raw := settingspec.Raw{settingspec.EnvSMTPHost: "smtp.example.com", settingspec.EnvSMTPPassword: "relay-credential"}
	value, sealed, err := s.encode(t.Context(), settingspec.GroupMail, raw)
	require.NoError(t, err)
	assert.NotContains(t, string(value), "relay-credential")
	assert.NotContains(t, string(sealed), "relay-credential")
	assert.Equal(t, keys.FormatVersion, sealed[0], "new secrets use the envelope format")
	back, err := s.decode(t.Context(), settingspec.GroupMail, value, sealed)
	require.NoError(t, err)
	assert.Equal(t, raw, back)
	_, err = s.decode(t.Context(), settingspec.GroupLogs, []byte(`{}`), sealed)
	require.ErrorIs(t, err, ErrUndecryptable, "the group is bound into the seal")
}
