package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

func newController(db support.FakeDB, migrations support.FakeMigrations, queries support.FakeQueries) *system.Controller {
	log := support.DiscardLogger()
	return system.NewController(
		system.NewHealthService(db, migrations, log),
		system.NewMetaService(queries, buildinfo.Info{Version: "v1", Commit: "c1"}),
	)
}

func TestControllerLiveness(t *testing.T) {
	c := newController(support.FakeDB{}, support.FakeMigrations{}, support.FakeQueries{})
	res, err := c.GetLiveness(context.Background(), gen.GetLivenessRequestObject{})
	require.NoError(t, err)
	body, ok := res.(gen.GetLiveness200JSONResponse)
	require.True(t, ok)
	assert.True(t, body.Success)
	require.NotNil(t, body.Message)
	assert.Equal(t, "system.health.live", *body.Message)
	require.NotNil(t, body.Data)
	assert.Equal(t, gen.HealthStatusStatus("ok"), body.Data.Status)
	assert.Nil(t, body.Error)
}

func TestControllerReadiness(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.En)
	t.Run("ready", func(t *testing.T) {
		c := newController(support.FakeDB{}, support.FakeMigrations{}, support.FakeQueries{})
		res, err := c.GetReadiness(ctx, gen.GetReadinessRequestObject{})
		require.NoError(t, err)
		body, ok := res.(gen.GetReadiness200JSONResponse)
		require.True(t, ok)
		assert.True(t, body.Success)
		assert.Nil(t, body.Error)
	})
	t.Run("unavailable carries data and a translated error", func(t *testing.T) {
		c := newController(support.FakeDB{Err: errors.New("down")}, support.FakeMigrations{}, support.FakeQueries{})
		res, err := c.GetReadiness(ctx, gen.GetReadinessRequestObject{})
		require.NoError(t, err)
		body, ok := res.(gen.GetReadiness503JSONResponse)
		require.True(t, ok)
		assert.False(t, body.Success)
		require.NotNil(t, body.Data)
		assert.Equal(t, gen.HealthStatusStatus("unavailable"), body.Data.Status)
		require.NotNil(t, body.Error)
		assert.Equal(t, apperrors.CodeServiceUnavailable, body.Error.Code)
		assert.Equal(t, "The service is not ready. Try again later.", body.Error.Message)
	})
}

func TestControllerMeta(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := newController(support.FakeDB{}, support.FakeMigrations{}, support.FakeQueries{})
		res, err := c.GetMeta(context.Background(), gen.GetMetaRequestObject{})
		require.NoError(t, err)
		body, ok := res.(gen.GetMeta200JSONResponse)
		require.True(t, ok)
		require.NotNil(t, body.Data)
		assert.Equal(t, gen.Meta{Version: "v1", Commit: "c1", ServerBuild: &gen.ServerBuild{Version: "v1", Commit: "c1"}, SetupRequired: true}, *body.Data)
		require.NotNil(t, body.Message)
		assert.Equal(t, "system.meta.loaded", *body.Message)
	})
	t.Run("service error is passed through as an AppError", func(t *testing.T) {
		c := newController(support.FakeDB{}, support.FakeMigrations{}, support.FakeQueries{Err: errors.New("boom")})
		res, err := c.GetMeta(context.Background(), gen.GetMetaRequestObject{})
		assert.Nil(t, res)
		require.Error(t, err)
		assert.Equal(t, apperrors.CodeInternal, apperrors.From(err).Code)
	})
}
