package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
	"github.com/storacha/project-agent/internal/similarity"
	"github.com/storacha/project-agent/internal/tasks"
)

func main() {
	ctx := context.Background()

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate required configuration for this command
	if cfg.GeminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable is required for PR linking")
	}

	// Get PR information from environment (passed from repository_dispatch event)
	prRepo := os.Getenv("PR_REPO")
	prNumberStr := os.Getenv("PR_NUMBER")
	prTitle := os.Getenv("PR_TITLE")
	prBody := os.Getenv("PR_BODY")

	if prRepo == "" || prNumberStr == "" {
		log.Fatalf("PR_REPO and PR_NUMBER environment variables are required")
	}

	prNumber, err := strconv.Atoi(prNumberStr)
	if err != nil {
		log.Fatalf("Invalid PR_NUMBER: %v", err)
	}

	// Parse owner/repo
	parts := strings.Split(prRepo, "/")
	if len(parts) != 2 {
		log.Fatalf("Invalid PR_REPO format, expected owner/repo: %s", prRepo)
	}
	prOwner := parts[0]
	prRepoName := parts[1]

	// Create GitHub client
	githubClient, err := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)
	if err != nil {
		log.Fatalf("Failed to create GitHub client: %v", err)
	}

	// Create similarity client
	similarityClient, err := similarity.NewClient(cfg.GeminiAPIKey)
	if err != nil {
		log.Fatalf("Failed to create similarity client: %v", err)
	}
	defer similarityClient.Close()

	log.Println("Starting PR-to-issue linking...")
	log.Printf("PR: %s/%s#%d", prOwner, prRepoName, prNumber)
	log.Printf("Title: %s", prTitle)

	// Run PR linking
	report, err := tasks.LinkPRToIssues(ctx, githubClient, similarityClient,
		prOwner, prRepoName, prNumber, prTitle, prBody, cfg)
	if err != nil {
		log.Fatalf("PR linking failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("PR LINKING REPORT")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("PR: %s/%s#%d\n\n", prOwner, prRepoName, prNumber)

	fmt.Printf("Direct References Found: %d\n", report.DirectReferencesFound)
	fmt.Printf("Issues Linked (Direct): %d\n", report.IssuesLinkedDirect)

	if report.SemanticMatchFound {
		fmt.Printf("Semantic Match Found: Yes\n")
		fmt.Printf("Issues Linked (Semantic): %d\n", report.IssueLinkedSemantic)
	} else {
		fmt.Printf("Semantic Match Found: No\n")
	}

	fmt.Printf("\nTotal Issues Moved to PR Review: %d\n", report.IssuesMovedToPRReview)

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		for _, errMsg := range report.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	log.Println("PR linking completed successfully")
}
