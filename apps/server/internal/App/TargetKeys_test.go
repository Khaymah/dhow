package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
)

type fixedStore struct {
	scope   string
	version int32
}

func (f fixedStore) Check(context.Context, *sqlc.Queries, secretstore.Request) (secretstore.CheckResult, error) {
	return secretstore.CheckResult{}, nil
}

func (f fixedStore) Resolve(_ context.Context, r secretstore.Request) ([]secretstore.Secret, error) {
	return []secretstore.Secret{{Name: r.Names[0], Scope: f.scope, Version: f.version, Value: secretstore.Value("the-key")}}, nil
}

func TestTheKeyOfATargetIsDeliveredOnlyAtTheVersionThatWasChecked(t *testing.T) {
	env := uuid.New()
	scope := pipelines.SecretScope{EnvironmentID: &env}
	pins := []environments.TargetPin{{Key: "WEB_SSH_KEY", Version: 2}}
	reserved := []string{"dhow.target.0.key"}
	cases := []struct {
		name  string
		store fixedStore
		want  error
	}{
		{"the version that was checked", fixedStore{scope: "environment", version: 2}, nil},
		{"a key rotated after the check", fixedStore{scope: "environment", version: 3}, jobs.ErrTargetChanged},
		{"a key of another scope", fixedStore{scope: "project", version: 2}, jobs.ErrTargetKeyMissing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &pipelineSecrets{store: c.store}
			out := map[string][]byte{}
			_, err := p.resolveTargetKeys(context.Background(), jobs.SecretRequest{}, scope, reserved, pins, out)
			if c.want != nil {
				require.ErrorIs(t, err, c.want)
				assert.Empty(t, out)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "the-key", string(out["dhow.target.0.key"]))
		})
	}
}

func TestPinsAreTheSameOnlyWhenKeyAndVersionMatch(t *testing.T) {
	a := []environments.TargetPin{{Key: "K", Version: 1}}
	assert.True(t, samePins(a, []environments.TargetPin{{Key: "K", Version: 1}}))
	assert.False(t, samePins(a, []environments.TargetPin{{Key: "K", Version: 2}}))
	assert.False(t, samePins(a, []environments.TargetPin{{Key: "L", Version: 1}}))
	assert.False(t, samePins(a, nil))
}
