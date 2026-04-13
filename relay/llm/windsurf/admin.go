package windsurf

import (
	"context"

	"github.com/iocgo/sdk/env"
)

func AdminValidateToken(token string) error {
	_, err := normalizeCredential(token)
	return err
}

func AdminFetchJWT(ctx context.Context, environment *env.Environment, token string) (string, error) {
	return genToken(ctx, environment, environment.GetString("server.proxied"), token)
}
