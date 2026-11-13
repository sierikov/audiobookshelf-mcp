package setup

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// maxErrorLen caps how much of a failed check's error is shown, in bytes,
// since a server or proxy may answer with a whole HTML page.
const maxErrorLen = 300

// VerifyWith checks the URL and token against a live Audiobookshelf server.
// It reads who the token belongs to (GET /api/me) and how many libraries that
// user can see (GET /api/libraries). It is the Verify that setup uses outside
// tests. Its errors never contain the token and are cut to maxErrorLen.
func VerifyWith(ctx context.Context, url, token string) (Account, error) {
	client := abs.NewClient(url, token)
	user, err := client.Me(ctx)
	if err != nil {
		return Account{}, shorten(withoutToken(fmt.Errorf("checking the token: %w", err), token))
	}
	libs, err := client.Libraries(ctx)
	if err != nil {
		return Account{}, shorten(withoutToken(fmt.Errorf("listing libraries: %w", err), token))
	}
	return Account{
		Username:  user.Username,
		Libraries: len(libs),
		CanUpdate: user.Permissions.Update,
	}, nil
}

// shorten cuts the message of err to maxErrorLen bytes plus "…", on a
// character boundary. A short error comes back unchanged. Mask the token
// first, so the cut cannot leave part of it behind.
func shorten(err error) error {
	msg := err.Error()
	if len(msg) <= maxErrorLen {
		return err
	}
	cut := maxErrorLen
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return errors.New(msg[:cut] + "…")
}
