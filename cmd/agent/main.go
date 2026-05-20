// Package main is the entry point for Yori (the Avagenc Gmail Agent)
// running as an AWS Lambda function.
//
// Build for Lambda deployment:
//
//	GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap cmd/agent/main.go
//
// Or for arm64 (Graviton — recommended for cost savings):
//
//	GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap cmd/agent/main.go
package main

import (
	"context"
	"log"

	"avagenc-gmail/internal/config"
	"avagenc-gmail/internal/db"
	"avagenc-gmail/internal/handler"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	// Cold-start work: validate config and warm the database pool so the
	// first invocation does not pay the connection cost.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[main] config load: %v", err)
	}

	if _, err := db.Pool(context.Background(), cfg.DatabaseURL); err != nil {
		log.Fatalf("[main] db pool init: %v", err)
	}

	log.Println("[main] Yori (Gmail Agent) ready")
	lambda.Start(handler.HandleRequest)
}
