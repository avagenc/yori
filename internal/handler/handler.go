// Package handler provides the AWS Lambda handler that orchestrates
// the Yori (Gmail Agent) execution flow within the Avagenc ecosystem.
package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"strings"

	"avagenc-gmail/internal/agent"
	"avagenc-gmail/internal/config"
	"avagenc-gmail/internal/db"

	"github.com/aws/aws-lambda-go/events"
	"go.naturallyfunny.dev/api"
	"go.naturallyfunny.dev/api/identity"
)

const (
	headerUserID    = "x-user-id"
	maxMessageBytes = 8 * 1024 // 8 KiB cap on user message body
)

// Request is the expected JSON payload from the API Gateway.
type Request struct {
	Message string `json:"message"`
}

// HandleRequest is the Lambda entry point.
// Flow:
//  1. Load memoised config.
//  2. Resolve x-user-id from headers and propagate it into ctx.
//  3. Decode and validate the request body.
//  4. Look up the user's gmail_connect row.
//  5. Refuse if scope is empty.
//  6. Run Yori with the user's command.
//  7. Return a structured Avagenc API response.
func HandleRequest(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("[handler] config load failed: %v", err)
		return errorResponse(api.NewError(api.Internal, "Internal configuration error")), nil
	}

	userID := extractUserID(event)
	if userID == "" {
		return errorResponse(api.NewError(api.Unauthenticated, "Missing required header: x-user-id")), nil
	}

	ctx, err = identity.NewContextWithUserID(ctx, userID)
	if err != nil {
		log.Printf("[handler] identity propagation failed: %v", err)
		return errorResponse(api.NewError(api.Internal, "Failed to propagate identity")), nil
	}

	body, err := decodeBody(event)
	if err != nil {
		log.Printf("[handler] body decode failed: %v", err)
		return errorResponse(api.NewError(api.InvalidArgument, "Invalid request body encoding")), nil
	}

	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("[handler] body parse failed: %v", err)
		return errorResponse(api.NewError(api.InvalidArgument, "Invalid request body: expected JSON with 'message'")), nil
	}

	req.Message = strings.TrimSpace(req.Message)
	switch {
	case req.Message == "":
		return errorResponse(api.NewError(api.InvalidArgument, "Missing required field: 'message'")), nil
	case len(req.Message) > maxMessageBytes:
		return errorResponse(api.NewError(api.InvalidArgument, "Field 'message' exceeds maximum allowed length")), nil
	}

	gmailConn, err := db.FetchGmailConnect(ctx, cfg.DatabaseURL, userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errorResponse(api.NewError(api.NotFound, "User gmail connection not found")), nil
		}
		log.Printf("[handler] db fetch failed: %v", err)
		return errorResponse(api.NewError(api.Internal, "Failed to load user gmail connection")), nil
	}

	if gmailConn.Scope == nil || *gmailConn.Scope == "" {
		return successResponse(map[string]any{
			"used_tool": []string{},
			"response":  "Kamu tidak memiliki akses ke gmail agent",
		}, "Access denied"), nil
	}

	result, err := agent.Run(ctx, agent.RunConfig{
		GeminiAPIKey:   cfg.GeminiAPIKey,
		Model:          cfg.Model,
		Command:        req.Message,
		RefreshToken:   gmailConn.RefreshToken,
		Scope:          *gmailConn.Scope,
		OwnerID:        userID,
		GmailBaseURL:   cfg.GmailBaseURL,
		ContactBaseURL: cfg.ContactBaseURL,
	})
	if err != nil {
		log.Printf("[handler] agent execution failed: %v", err)
		return errorResponse(api.NewError(api.Internal, "Agent execution failed")), nil
	}

	return successResponse(result, "Message Processed"), nil
}

// extractUserID resolves x-user-id from API Gateway's two header maps,
// case-insensitively. Returns "" when not present.
func extractUserID(event events.APIGatewayProxyRequest) string {
	for k, v := range event.Headers {
		if strings.EqualFold(k, headerUserID) {
			return v
		}
	}
	for k, vs := range event.MultiValueHeaders {
		if strings.EqualFold(k, headerUserID) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// decodeBody returns the raw request body, decoding base64 when API
// Gateway flagged the payload as binary.
func decodeBody(event events.APIGatewayProxyRequest) ([]byte, error) {
	if event.IsBase64Encoded {
		return base64.StdEncoding.DecodeString(event.Body)
	}
	return []byte(event.Body), nil
}

// successResponse builds an Avagenc-style success envelope.
func successResponse(data any, message string) events.APIGatewayProxyResponse {
	recorder := httptest.NewRecorder()
	api.WriteSuccess(recorder, api.OK, message, data, nil)
	return events.APIGatewayProxyResponse{
		StatusCode: recorder.Code,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: recorder.Body.String(),
	}
}

// errorResponse builds an Avagenc-style error envelope. The handler never
// returns a Go error to Lambda — that would trigger an automatic retry —
// so all failures travel back as structured HTTP responses.
func errorResponse(apiErr *api.Error) events.APIGatewayProxyResponse {
	recorder := httptest.NewRecorder()
	api.WriteError(recorder, apiErr)
	return events.APIGatewayProxyResponse{
		StatusCode: recorder.Code,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: recorder.Body.String(),
	}
}
