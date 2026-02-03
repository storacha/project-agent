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

	log.Println("Starting duplicate detection...")
	log.Printf("Organization: %s", cfg.GithubOrg)
	log.Printf("Project Number: %d", cfg.ProjectNumber)
	log.Printf("Similarity Threshold: %.0f%%", cfg.DuplicateSimilarity*100)
	log.Printf("Target Statuses: %v", cfg.TargetStatuses)

	// Fetch issues with target statuses
	log.Printf("Fetching issues with statuses: %v\n", cfg.TargetStatuses)
	issues, err := githubClient.GetIssuesByStatuses(ctx, cfg.TargetStatuses)
	if err != nil {
		log.Fatalf("Failed to fetch backlog issues: %v", err)
	}

	log.Printf("Found %d issues with target statuses\n", len(issues))

	if len(issues) < 2 {
		log.Println("Need at least 2 issues to detect duplicates")
		return
	}

	// Run duplicate detection
	report, err := tasks.DetectDuplicates(ctx, githubClient, similarityClient, issues, cfg)
	if err != nil {
		log.Fatalf("Duplicate detection failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("DUPLICATE DETECTION REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Run Date: %s\n\n", time.Now().Format(time.RFC3339))

	fmt.Printf("Issues Analyzed: %d\n", report.IssuesAnalyzed)
	fmt.Printf("Potential Duplicates Found: %d groups\n", len(report.DuplicateGroups))
	fmt.Printf("Issues Labeled: %d\n", report.IssuesLabeled)

	if len(report.DuplicateGroups) > 0 {
		fmt.Println("\nDuplicate Groups:")
		for i, group := range report.DuplicateGroups {
			fmt.Printf("\n  Group %d (similarity: %.2f):\n", i+1, group.Similarity)
			for _, issue := range group.Issues {
				fmt.Printf("    - #%d: %s\n", issue.Number, issue.Title)
			}
		}
	}

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		for _, errMsg := range report.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	log.Println("Duplicate detection completed successfully")
}
