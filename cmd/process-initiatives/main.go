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

	log.Println("Starting initiative processing...")
	log.Printf("Organization: %s", cfg.GithubOrg)
	log.Printf("Project Number: %d", cfg.ProjectNumber)
	if cfg.DryRun {
		log.Println("DRY RUN MODE: No changes will be made")
	}

	// Fetch all Initiative-type issues
	log.Println("Fetching all Initiative-type issues from project...")
	initiatives, err := githubClient.GetInitiativeIssues(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch Initiative issues: %v", err)
	}

	log.Printf("Found %d Initiative issues\n", len(initiatives))

	if len(initiatives) == 0 {
		log.Println("No Initiative issues found, nothing to do")
		return
	}

	// Process initiatives and their sub-issues
	report, err := tasks.ProcessInitiatives(ctx, githubClient, initiatives, cfg)
	if err != nil {
		log.Fatalf("Initiative processing failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("INITIATIVE PROCESSING REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Run Date: %s\n\n", time.Now().Format(time.RFC3339))

	fmt.Printf("Initiatives Processed: %d\n", report.InitiativesProcessed)
	fmt.Printf("Sub-issues Found (total): %d\n", report.SubIssuesFound)
	fmt.Printf("Sub-issues Added to Project: %d\n", report.SubIssuesAdded)
	fmt.Printf("Sub-issues Updated (Initiative field): %d\n", report.SubIssuesUpdated)

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		for _, errMsg := range report.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	log.Println("Initiative processing completed successfully")
}
