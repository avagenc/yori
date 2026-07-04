package yori

import (
	_ "embed"
	"fmt"

	adkspotify "go.naturallyfunny.dev/adk/spotify"
	"go.naturallyfunny.dev/spotify"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	adktool "google.golang.org/adk/tool"
)

// Config holds the dependencies a consumer must supply. Yori's identity and
// system instruction are owned by the module, not configured here. Per-channel
// or per-run instruction is the consumer's concern — append it from a
// llmagent.BeforeModelCallback / plugin on your own runner.
type Config struct {
	Model model.LLM
	// SpotifyClient is the per-user Spotify client backing search and playback.
	// One OAuth refresh token per user covers every capability; the client
	// resolves the token from its TokenStore on each call.
	SpotifyClient *spotify.Client
	// AdditionalInstruction is appended to Yori's base system instruction.
	// Use it to supply channel-specific or deployment-specific context that
	// the module itself cannot know.
	AdditionalInstruction string
}

//go:embed internal/description.txt
var description string

//go:embed internal/instruction.txt
var systemInstruction string

// New builds the Yori agent — a Spotify music LLM agent that can search
// tracks, browse the human's playlists, and drive playback on their devices.
// Running it (runner, session, and any per-run instruction) is the consumer's
// responsibility.
func New(cfg Config) (agent.Agent, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("yori: model is required")
	}
	if cfg.SpotifyClient == nil {
		return nil, fmt.Errorf("yori: spotify client is required")
	}

	spotifyTools, err := adkspotify.Tools(cfg.SpotifyClient)
	if err != nil {
		return nil, fmt.Errorf("yori: spotify tools: %w", err)
	}

	tools := make([]adktool.Tool, 0, len(spotifyTools))
	tools = append(tools, spotifyTools...)

	instruction := "[SYSTEM_INSTRUCTION]" + systemInstruction + "\n[/SYSTEM_INSTRUCTION]"
	if cfg.AdditionalInstruction != "" {
		instruction = "[SYSTEM_INSTRUCTION]" + systemInstruction + "\n\n" + cfg.AdditionalInstruction + "\n[/SYSTEM_INSTRUCTION]"
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "yori",
		Model:       cfg.Model,
		Tools:       tools,
		Description: description,
		Instruction: instruction,
	})
	if err != nil {
		return nil, fmt.Errorf("yori: agent: %w", err)
	}
	return a, nil
}
