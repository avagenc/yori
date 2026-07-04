package main

import (
	"context"
	"log"
	"os"

	_ "time/tzdata"

	"github.com/joho/godotenv"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"google.golang.org/genai"

	"go.naturallyfunny.dev/spotify"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/console"
	"google.golang.org/adk/cmd/launcher/universal"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/session"

	yori "go.avagenc.com/yori"
)

// staticTokenStore hands out a single fixed refresh token to any user,
// obtained once via `go run ./cmd/connect`. No postgres/firestore needed for
// local dev, and whatever user_id the session starts with is treated as
// connected.
type staticTokenStore struct {
	refreshToken string
}

func (s *staticTokenStore) GetRefreshToken(_ context.Context, _ string) (string, error) {
	if s.refreshToken == "" {
		return "", spotify.ErrNotConnected
	}
	return s.refreshToken, nil
}
func (s *staticTokenStore) SaveRefreshToken(_ context.Context, _, _ string) error { return nil }
func (s *staticTokenStore) DeleteRefreshToken(_ context.Context, _ string) error  { return nil }

func main() {
	// 0. Load environment vars
	if err := godotenv.Load(); err != nil {
		log.Println("info: .env file not found, using system environment variables")
	}
	// 1. Spotify client
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
		// Frontend callback page Spotify redirects to after consent. Fixed, not
		// env: the same dev address every time, registered verbatim on the app
		// in the Spotify Developer Dashboard. cmd/connect serves this path to
		// capture the dev refresh token.
		spotifyauth.WithRedirectURL("http://127.0.0.1:5173/spotify/callback"),
		spotifyauth.WithScopes(spotify.RequiredScopes...),
	)
	devRefreshToken := os.Getenv("DEV_SPOTIFY_REFRESH_TOKEN")
	if devRefreshToken == "" {
		log.Fatal("fatal: DEV_SPOTIFY_REFRESH_TOKEN is required (obtain it with `go run ./cmd/connect`)")
	}
	spotifyClient := spotify.New(&staticTokenStore{refreshToken: devRefreshToken}, auth)
	// 2. Model
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("fatal: GEMINI_API_KEY is required")
	}
	agentModel, err := gemini.NewModel(context.Background(), "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: geminiAPIKey,
	})
	if err != nil {
		log.Fatalf("fatal: build gemini model: %v", err)
	}
	// 3. Agent
	yoriAgent, err := yori.New(yori.Config{
		Model:         agentModel,
		SpotifyClient: spotifyClient,
	})
	if err != nil {
		log.Fatalf("fatal: build yori agent: %v", err)
	}
	// 4. Launcher — console is ADK's CLI run mode (equivalent to Python's
	// `adk run`). Prepend the "console" keyword so any extra flags
	// (e.g. -streaming_mode) are forwarded to it.
	config := &launcher.Config{
		AgentLoader:    agent.NewSingleLoader(yoriAgent),
		SessionService: session.InMemoryService(),
	}
	l := universal.NewLauncher(console.NewLauncher())
	args := append([]string{"console"}, os.Args[1:]...)
	if err = l.Execute(context.Background(), config, args); err != nil {
		log.Fatalf("fatal: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
