package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
	"github.com/storacha/project-agent/internal/tasks"
)

func main() {
	ctx := context.Background()

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create GitHub client
	githubClient, err := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)
	if err != nil {
		log.Fatalf("Failed to create GitHub client: %v", err)
	}

	log.Println("Starting stale issue triage...")
	log.Printf("Organization: %s", cfg.GithubOrg)
	log.Printf("Project Number: %d", cfg.ProjectNumber)
	log.Printf("Staleness Threshold: %d days", cfg.StalenessThresholdDays)
	log.Printf("Target Statuses: %v", cfg.TargetStatuses)

	// Fetch issues with target statuses
	log.Printf("Fetching issues with statuses: %v\n", cfg.TargetStatuses)
	issues, err := githubClient.GetIssuesByStatuses(ctx, cfg.TargetStatuses)
	if err != nil {
		log.Fatalf("Failed to fetch backlog issues: %v", err)
	}

	log.Printf("Found %d issues with target statuses\n", len(issues))

	if len(issues) == 0 {
		log.Println("No issues found with target statuses, nothing to do")
		return
	}

	// Run stale issue triage
	report, err := tasks.TriageStaleIssues(ctx, githubClient, issues, cfg)
	if err != nil {
		log.Fatalf("Triage failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("STALE ISSUE TRIAGE REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Run Date: %s\n\n", time.Now().Format(time.RFC3339))

	fmt.Printf("Issues Analyzed: %d\n", report.IssuesAnalyzed)
	fmt.Printf("Stale Issues Found: %d\n", report.StaleIssuesFound)
	fmt.Printf("Issues Moved to Stuck/Dead: %d\n", report.IssuesMoved)

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		for _, errMsg := range report.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	log.Println("Triage completed successfully")
}
