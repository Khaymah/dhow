package runnerauth

import (
	"context"
	"net/http"

	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func CheckForTest(ctx context.Context, g *Gate, table *policy.Table, procedure string, header http.Header) (context.Context, error) {
	return g.check(ctx, table, http.MethodPost, procedure, header)
}
