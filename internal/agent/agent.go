// Package agent provides Yori, the Gmail specialist agent in the Avagenc
// ecosystem. It uses Google GenAI (Gemini) with function calling to translate
// natural-language commands from Ava (the orchestrator) into Gmail HTTP tool
// invocations.
package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"avagenc-gmail/internal/tools"

	"google.golang.org/genai"
)

// maxToolIterations bounds the agent loop so a misbehaving model cannot
// spin forever.
const maxToolIterations = 10

// Result is the structured output of a single agent invocation.
type Result struct {
	Response  string   `json:"response"`
	UsageTool []string `json:"used_tool"`
}

// RunConfig contains all parameters needed for a single agent run.
type RunConfig struct {
	GeminiAPIKey   string
	Model          string
	Command        string
	RefreshToken   string
	Scope          string
	OwnerID        string
	GmailBaseURL   string
	ContactBaseURL string
}

// Run executes Yori for a single command. It opens a GenAI session, sends
// the command with system instructions, and drives the tool-calling loop
// until the model produces a final text response.
func Run(ctx context.Context, cfg RunConfig) (*Result, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			APIVersion: "v1beta",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("agent: create GenAI client: %w", err)
	}

	contents := []*genai.Content{
		{
			Role:  "user",
			Parts: []*genai.Part{{Text: buildUserMessage(cfg.Command)}},
		},
	}

	genConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: buildSystemInstruction(cfg.Scope)}},
		},
		Tools: tools.GmailTools(),
	}

	executor := tools.NewExecutor(cfg.GmailBaseURL, cfg.ContactBaseURL, cfg.RefreshToken, cfg.OwnerID)

	// Initialise as an empty (non-nil) slice so the JSON encoding is "[]"
	// rather than "null" when no tools are invoked.
	usedTools := make([]string, 0)

	for i := 0; i < maxToolIterations; i++ {
		log.Printf("[agent] iteration %d/%d", i+1, maxToolIterations)

		resp, err := client.Models.GenerateContent(ctx, cfg.Model, contents, genConfig)
		if err != nil {
			return nil, fmt.Errorf("agent: GenerateContent at iteration %d: %w", i+1, err)
		}

		if len(resp.Candidates) == 0 {
			return nil, fmt.Errorf("agent: no candidates at iteration %d", i+1)
		}
		candidate := resp.Candidates[0]
		if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
			return nil, fmt.Errorf("agent: empty content at iteration %d", i+1)
		}

		functionCalls := extractFunctionCalls(candidate.Content.Parts)

		if len(functionCalls) == 0 {
			finalText := extractText(candidate.Content.Parts)
			if finalText == "" {
				return nil, fmt.Errorf("agent: model returned neither function calls nor text at iteration %d", i+1)
			}
			return &Result{
				Response:  finalText,
				UsageTool: usedTools,
			}, nil
		}

		contents = append(contents, candidate.Content)

		functionResponseParts := make([]*genai.Part, 0, len(functionCalls))
		for _, fc := range functionCalls {
			// Names only — never log args, which contain user PII.
			log.Printf("[agent] tool=%s", fc.Name)
			usedTools = append(usedTools, fc.Name)

			result, err := executor.Execute(ctx, fc.Name, fc.Args)
			if err != nil {
				log.Printf("[agent] tool=%s execution error: %v", fc.Name, err)
				result = map[string]any{"error": err.Error()}
			}

			functionResponseParts = append(functionResponseParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: result,
				},
			})
		}

		contents = append(contents, &genai.Content{
			Role:  "user",
			Parts: functionResponseParts,
		})
	}

	return nil, fmt.Errorf("agent: exceeded maximum tool iterations (%d)", maxToolIterations)
}

// buildSystemInstruction constructs Yori's XML-tagged system prompt. The
// XML structure mirrors the convention used by Ava across the Avagenc
// ecosystem so model attention is anchored on rule boundaries.
func buildSystemInstruction(scope string) string {
	currentTime := time.Now().UTC().Format(time.RFC3339)

	return fmt.Sprintf(`<role>
You are Yori, the Gmail specialist agent in the Avagenc ecosystem. You execute Gmail and contact-list operations on behalf of Ava (the orchestrator) by invoking the Avagenc Gmail HTTP tools.
</role>

<context>
  <current_time>%s</current_time>
  <user_scopes>%s</user_scopes>
</context>

<rules>
  <rule>You receive commands from Ava. You do not converse with end users.</rule>
  <rule>Execute exactly what is requested. Never infer, guess, or fabricate missing data.</rule>
  <rule>Match request parameters exactly. Never omit required fields. Never add extra fields.</rule>
  <rule>If required data is missing, return a short text response naming the missing fields. Do NOT call the tool.</rule>
  <rule>Reply in the same language as the incoming command. Indonesian replies use natural Indonesian; keep technical terms in English.</rule>
  <rule>Operate strictly within the scopes listed in &lt;user_scopes&gt;. The backend also enforces this — calls outside scope will be rejected.</rule>
  <rule>The refresh_token is injected by the backend; never include it in tool arguments.</rule>
</rules>

<tools>
  <tool name="gmail-read">Fetch Gmail messages with optional filters. Return summary or required information from the result.</tool>
  <tool name="gmail-send">Send an email. Requires receiver, title, body. Do NOT call unless all three are present.</tool>
  <tool name="gmail-get-lables">Fetch the user's Gmail labels.</tool>
  <tool name="gmail-apply-lable">Apply a label to a message. Requires message_id and label_id.</tool>
  <tool name="gmail-create-lable">Create a label with a custom name.</tool>
  <tool name="gmail-get-messages-by-lable">Fetch messages filtered by label_id.</tool>
  <tool name="get_user_contact">Look up a contact's gmail or phone by name when the command provides a name without contact details.</tool>
  <tool name="add_user_contact">Add a new contact. Requires name plus at least one of gmail or number.</tool>
</tools>

<gmail_read_filters>
  <filter name="query">Free-text search (Gmail search syntax).</filter>
  <filter name="from">Sender email.</filter>
  <filter name="label">Label name or ID.</filter>
  <filter name="after">Date after, format YYYY/MM/DD.</filter>
  <filter name="before">Date before, format YYYY/MM/DD.</filter>
  <filter name="max_results">Integer as string. Default "10".</filter>
  <note>If a filter is not explicitly provided, pass empty string "".</note>
</gmail_read_filters>

<security>
  <rule>Never modify email content unless explicitly instructed.</rule>
  <rule>Never send emails unless receiver, title, and body are all present.</rule>
  <rule>Never call tools outside the user's enabled scopes.</rule>
</security>`, currentTime, scope)
}

// buildUserMessage wraps the raw command in an XML envelope so the model
// can clearly distinguish the orchestrator's instruction from any other
// content that may appear in the conversation.
func buildUserMessage(command string) string {
	return fmt.Sprintf("<command source=\"main_agent\">\n%s\n</command>", command)
}

func extractFunctionCalls(parts []*genai.Part) []*genai.FunctionCall {
	calls := make([]*genai.FunctionCall, 0, len(parts))
	for _, part := range parts {
		if part.FunctionCall != nil {
			calls = append(calls, part.FunctionCall)
		}
	}
	return calls
}

func extractText(parts []*genai.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(part.Text)
	}
	return b.String()
}
