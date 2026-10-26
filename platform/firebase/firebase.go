package firebase

/**
setup go https://firebase.google.com/docs/admin/setup#go
verify: https://github.com/firebase/firebase-admin-go/blob/3be3a8604dad5dba6717295c68882c7b98ba3df3/snippets/auth.go#L82-L92

*/
import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"xxxdonacion/platform/config"
)

type Identity struct {
	UID           string
	Email         string
	EmailVerified bool
	Name          string
}

type Verifier interface {
	Verify(ctx context.Context, idToken string) (*Identity, error)
}

// RevokedVerifier melakukan pengecekan revocation/disabled ke server Google.
// Dipakai hanya untuk endpoint sensitif karena ada RPC tambahan.
type RevokedVerifier interface {
	VerifyRevoked(ctx context.Context, idToken string) (*Identity, error)
}

// Client membungkus Admin SDK Auth untuk verifikasi token.
// Operasi admin (create/disable/delete) ada di module identity, bukan di sini.
type Client struct {
	client *auth.Client
}

func NewClient(ctx context.Context, cfg *config.Firebase) (*Client, error) {
	opt := option.WithAuthCredentialsFile(option.ServiceAccount, cfg.CredentialsFile)
	c := &firebase.Config{ProjectID: cfg.ProjectID}

	app, err := firebase.NewApp(ctx, c, opt)
	if err != nil {
		return nil, fmt.Errorf("init firebase app: %w", err)
	}

	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("init firebase client: %w", err)
	}

	return &Client{client: client}, nil
}

// AuthClient dipakai module identity untuk operasi user management.
func (c *Client) AuthClient() *auth.Client { return c.client }

// Verify memverifikasi signature, expiry, issuer, dan audience secara lokal
// (public key Google di-cache), jadi tidak ada network call per request.
// Sengaja TIDAK memakai VerifyIDTokenAndCheckRevoked: itu memanggil server
// Google tiap request. Revocation kita tangani lewat status di DB/Redis.
func (c *Client) Verify(ctx context.Context, idToken string) (*Identity, error) {
	token, err := c.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("verifying id token: %w", err)
	}
	return identityFromToken(token), nil
}

func (c *Client) VerifyRevoked(ctx context.Context, idToken string) (*Identity, error) {
	token, err := c.client.VerifyIDTokenAndCheckRevoked(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("verifying id token with revocation check: %w", err)
	}
	return identityFromToken(token), nil
}

func identityFromToken(token *auth.Token) *Identity {
	email, _ := token.Claims["email"].(string)
	name, _ := token.Claims["name"].(string)
	verified, _ := token.Claims["email_verified"].(bool)
	return &Identity{
		UID:           token.UID,
		Email:         email,
		EmailVerified: verified,
		Name:          name,
	}
}
