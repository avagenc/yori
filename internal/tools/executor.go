package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// defaultMaxResults is the upper bound applied when a tool call omits
// max_results or supplies an unparseable value.
const defaultMaxResults = 10

// httpTimeout is the per-request budget for downstream microservice calls.
const httpTimeout = 30 * time.Second

// Executor handles the actual execution of tool calls by dispatching
// HTTP requests to the Gmail API server and Contact API server.
type Executor struct {
	gmailBaseURL   string
	contactBaseURL string
	refreshToken   string
	ownerID        string
	httpClient     *http.Client
}

// NewExecutor creates a new tool executor with the given Gmail server
// base URL, contact base URL, user refresh token, and owner ID.
func NewExecutor(gmailBaseURL, contactBaseURL, refreshToken, ownerID string) *Executor {
	return &Executor{
		gmailBaseURL:   gmailBaseURL,
		contactBaseURL: contactBaseURL,
		refreshToken:   refreshToken,
		ownerID:        ownerID,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// Execute dispatches a tool call by function name and arguments,
// returning the result as a map suitable for genai.FunctionResponse.
func (e *Executor) Execute(ctx context.Context, funcName string, args map[string]any) (map[string]any, error) {
	switch funcName {
	case ToolNameGmailRead:
		return e.executeGmailRead(ctx, args)
	case ToolNameGmailSend:
		return e.executeGmailSend(ctx, args)
	case ToolNameGmailGetLables:
		return e.executeGmailGetLables(ctx, args)
	case ToolNameGmailApplyLable:
		return e.executeGmailApplyLable(ctx, args)
	case ToolNameGmailCreateLable:
		return e.executeGmailCreateLable(ctx, args)
	case ToolNameGmailGetMessagesByLable:
		return e.executeGmailGetMessagesByLable(ctx, args)
	case ToolNameGetUserContact:
		return e.executeGetUserContact(ctx, args)
	case ToolNameAddUserContact:
		return e.executeAddUserContact(ctx, args)
	default:
		return nil, fmt.Errorf("executor: unknown tool function: %s", funcName)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Gmail Tool Implementations
// ─────────────────────────────────────────────────────────────────────────────

// executeGmailRead calls POST /gmail/get
func (e *Executor) executeGmailRead(ctx context.Context, args map[string]any) (map[string]any, error) {
	body := map[string]any{
		"refreshToken": e.refreshToken,
	}

	// Only include optional fields if they have non-empty values
	if v := getStringArg(args, "query", ""); v != "" {
		body["query"] = v
	}
	if v := getStringArg(args, "from", ""); v != "" {
		body["from"] = v
	}
	if v := getStringArg(args, "label", ""); v != "" {
		body["label"] = v
	}
	if v := getStringArg(args, "after", ""); v != "" {
		body["after"] = v
	}
	if v := getStringArg(args, "before", ""); v != "" {
		body["before"] = v
	}

	body["maxResults"] = parseMaxResults(args)

	result, err := e.postGmailJSON(ctx, "/gmail/get", body)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"emails": result}, nil
}

// executeGmailSend calls POST /gmail/send
func (e *Executor) executeGmailSend(ctx context.Context, args map[string]any) (map[string]any, error) {
	receiver := getStringArg(args, "receiver", "")
	title := getStringArg(args, "title", "")
	bodyText := getStringArg(args, "body", "")

	if receiver == "" {
		return map[string]any{"error": "receiver email is required"}, nil
	}
	if title == "" {
		return map[string]any{"error": "email subject (title) is required"}, nil
	}
	if bodyText == "" {
		return map[string]any{"error": "email body is required"}, nil
	}

	body := map[string]any{
		"refreshToken": e.refreshToken,
		"to":           receiver,
		"subject":      title,
		"body":         bodyText,
	}

	result, err := e.postGmailJSON(ctx, "/gmail/send", body)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"result": result}, nil
}

// executeGmailGetLables calls POST /gmail/lables with header tool: list
func (e *Executor) executeGmailGetLables(ctx context.Context, args map[string]any) (map[string]any, error) {
	body := map[string]any{
		"refreshToken": e.refreshToken,
	}

	result, err := e.postGmailLables(ctx, "list", body)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"labels": result}, nil
}

// executeGmailApplyLable calls POST /gmail/lables with header tool: apply
func (e *Executor) executeGmailApplyLable(ctx context.Context, args map[string]any) (map[string]any, error) {
	messageID := getStringArg(args, "message_id", "")
	labelID := getStringArg(args, "label_id", "")

	if messageID == "" {
		return map[string]any{"error": "message_id is required"}, nil
	}
	if labelID == "" {
		return map[string]any{"error": "label_id is required"}, nil
	}

	body := map[string]any{
		"refreshToken": e.refreshToken,
		"messageId":    messageID,
		"labelId":      labelID,
	}

	result, err := e.postGmailLables(ctx, "apply", body)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"result": result}, nil
}

// executeGmailCreateLable calls POST /gmail/lables with header tool: create
func (e *Executor) executeGmailCreateLable(ctx context.Context, args map[string]any) (map[string]any, error) {
	name := getStringArg(args, "name", "")
	if name == "" {
		return map[string]any{"error": "label name is required"}, nil
	}

	body := map[string]any{
		"refreshToken": e.refreshToken,
		"name":         name,
	}

	// Include optional styling fields only if provided
	if v := getStringArg(args, "label_list_visibility", ""); v != "" {
		body["labelListVisibility"] = v
	}
	if v := getStringArg(args, "message_list_visibility", ""); v != "" {
		body["messageListVisibility"] = v
	}
	if v := getStringArg(args, "text_color", ""); v != "" {
		body["textColor"] = v
	}
	if v := getStringArg(args, "background_color", ""); v != "" {
		body["backgroundColor"] = v
	}

	result, err := e.postGmailLables(ctx, "create", body)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"result": result}, nil
}

// executeGmailGetMessagesByLable calls POST /gmail/lables with header tool: get-message
func (e *Executor) executeGmailGetMessagesByLable(ctx context.Context, args map[string]any) (map[string]any, error) {
	labelID := getStringArg(args, "label_id", "")
	if labelID == "" {
		return map[string]any{"error": "label_id is required"}, nil
	}

	body := map[string]any{
		"refreshToken": e.refreshToken,
		"labelId":      labelID,
	}

	body["maxResults"] = parseMaxResults(args)

	result, err := e.postGmailLables(ctx, "get-message", body)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"messages": result}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Contact Tool Implementations
// ─────────────────────────────────────────────────────────────────────────────

// executeGetUserContact calls GET [CONTACT_BASE_URL]/get with owner_id
func (e *Executor) executeGetUserContact(ctx context.Context, args map[string]any) (map[string]any, error) {
	body := map[string]any{
		"owner_id": e.ownerID,
	}

	url := e.contactBaseURL + "/get"
	result, err := e.executeJSONRequest(ctx, http.MethodGet, url, body, nil)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"contacts": result}, nil
}

// executeAddUserContact calls POST [CONTACT_BASE_URL]/create
func (e *Executor) executeAddUserContact(ctx context.Context, args map[string]any) (map[string]any, error) {
	name := getStringArg(args, "name", "")
	gmail := getStringArg(args, "gmail", "")
	number := getStringArg(args, "number", "")

	if name == "" {
		return map[string]any{"error": "name parameter is required"}, nil
	}
	if gmail == "" && number == "" {
		return map[string]any{"error": "either gmail or number must be provided"}, nil
	}

	body := map[string]any{
		"owner_id": e.ownerID,
		"name":     name,
	}
	// Only include gmail/number if actually provided
	if gmail != "" {
		body["gmail"] = gmail
	}
	if number != "" {
		body["number"] = number
	}

	url := e.contactBaseURL + "/create"
	result, err := e.executeJSONRequest(ctx, http.MethodPost, url, body, nil)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}

	return map[string]any{"result": result}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP Helpers
// ─────────────────────────────────────────────────────────────────────────────

// postGmailJSON is a convenience wrapper for POST to Gmail API endpoints.
func (e *Executor) postGmailJSON(ctx context.Context, path string, body map[string]any) (any, error) {
	url := e.gmailBaseURL + path
	return e.executeJSONRequest(ctx, http.MethodPost, url, body, nil)
}

// postGmailLables sends a POST to /gmail/lables with the required "tool" header.
func (e *Executor) postGmailLables(ctx context.Context, toolHeader string, body map[string]any) (any, error) {
	url := e.gmailBaseURL + "/gmail/lables"
	headers := map[string]string{
		"tool": toolHeader,
	}
	return e.executeJSONRequest(ctx, http.MethodPost, url, body, headers)
}

// executeJSONRequest creates and dispatches an HTTP request with a JSON body.
// extraHeaders is an optional map of additional headers to set.
func (e *Executor) executeJSONRequest(ctx context.Context, method string, url string, body map[string]any, extraHeaders map[string]string) (any, error) {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("executor: failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("executor: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Apply any extra headers (e.g., "tool" header for label endpoints)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executor: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("executor: failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("executor: API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		// If the response isn't valid JSON, return it as a string
		return string(respBody), nil
	}

	return result, nil
}

// getStringArg safely extracts a string argument from the model's args map.
// Returns defaultVal if the key is absent, of a non-string type, or empty.
func getStringArg(args map[string]any, key, defaultVal string) string {
	val, ok := args[key]
	if !ok {
		return defaultVal
	}
	str, ok := val.(string)
	if !ok {
		return defaultVal
	}
	if str == "" {
		return defaultVal
	}
	return str
}

// parseMaxResults extracts and validates max_results from tool args,
// falling back to defaultMaxResults when missing or unparseable.
func parseMaxResults(args map[string]any) int {
	raw := getStringArg(args, "max_results", "")
	if raw == "" {
		return defaultMaxResults
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultMaxResults
	}
	return parsed
}
