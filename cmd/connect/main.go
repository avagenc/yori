// cmd/connect is a one-off local helper to obtain a Spotify OAuth refresh
// token for the dev entry points (cmd/cli). Spotify OAuth has no dev bypass —
// this runs the real consent flow once via the loopback redirect pattern and
// prints the resulting refresh token to paste into .env as
// DEV_SPOTIFY_REFRESH_TOKEN.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/joho/godotenv"
	spotifyauth "github.com/zmb3/spotify/v2/auth"

	"go.naturallyfunny.dev/spotify"
)

// noopTokenStore satisfies spotify.TokenStore for a client that will only ever
// call Exchange, never Connect — so no token is ever actually stored.
type noopTokenStore struct{}

func (noopTokenStore) GetRefreshToken(context.Context, string) (string, error) {
	return "", spotify.ErrNotConnected
}
func (noopTokenStore) SaveRefreshToken(context.Context, string, string) error { return nil }
func (noopTokenStore) DeleteRefreshToken(context.Context, string) error       { return nil }

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("fatal: generate state: %v", err)
	}
	return hex.EncodeToString(b)
}

func main() {
	// 0. Load environment vars
	if err := godotenv.Load(); err != nil {
		log.Println("info: .env file not found, using system environment variables")
	}
	// 1. Spotify client. The redirect URL is fixed, not env: it must be
	// registered verbatim on the app in the Spotify Developer Dashboard, and
	// this helper serves its path on the loopback to catch the code. Keep it in
	// sync with cmd/cli.
	const redirectURL = "http://127.0.0.1:5173/spotify/callback"
	u, err := url.Parse(redirectURL)
	if err != nil {
		log.Fatalf("fatal: parse redirect URL: %v", err)
	}
	spotifyClientID := os.Getenv("SPOTIFY_CLIENT_ID")
	if spotifyClientID == "" {
		log.Fatal("fatal: SPOTIFY_CLIENT_ID is required")
	}
	spotifyClientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if spotifyClientSecret == "" {
		log.Fatal("fatal: SPOTIFY_CLIENT_SECRET is required")
	}
	auth := spotifyauth.New(
		spotifyauth.WithClientID(spotifyClientID),
		spotifyauth.WithClientSecret(spotifyClientSecret),
		spotifyauth.WithRedirectURL(redirectURL),
		spotifyauth.WithScopes(spotify.RequiredScopes...),
	)
	client := spotify.New(noopTokenStore{}, auth)
	state := randomState()

	// 2. Loopback server to catch the callback
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		if got := r.URL.Query().Get("state"); got != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			log.Printf("fatal: state mismatch: got %q want %q", got, state)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			http.Error(w, "access denied", http.StatusBadRequest)
			log.Printf("fatal: authorization denied: %s", e)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			log.Println("fatal: callback carried no code")
			return
		}

		refreshToken, err := client.Exchange(r.Context(), code)
		if err != nil {
			var scopeErr *spotify.ScopeError
			if errors.As(err, &scopeErr) {
				http.Error(w, "missing scopes — reconnect and approve all permissions", http.StatusBadRequest)
				log.Printf("fatal: missing scopes %v (granted %v)", scopeErr.Missing, scopeErr.Granted)
				return
			}
			http.Error(w, "exchange failed", http.StatusInternalServerError)
			log.Printf("fatal: exchange: %v", err)
			return
		}

		fmt.Fprint(w, "Connected. You can close this tab.")
		fmt.Println()
		fmt.Println("Paste this into yori's .env as DEV_SPOTIFY_REFRESH_TOKEN:")
		fmt.Println()
		fmt.Println(refreshToken)
		fmt.Println()
	})

	srv := &http.Server{Addr: u.Host, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("fatal: local server: %v", err)
		}
	}()

	fmt.Println("Open this URL in a browser and grant access:")
	fmt.Println()
	fmt.Println(client.AuthURL(state))
	fmt.Println()

	<-done
	_ = srv.Shutdown(context.Background())
}
