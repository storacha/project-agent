package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/discord"
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

	log.Println("Starting daily update check...")
	log.Printf("Organization: %s\n", cfg.GithubOrg)
	log.Printf("Project: %d\n", cfg.ProjectNumber)
	log.Printf("Threshold: %d days\n", cfg.DailyUpdateThreshold)
	if cfg.DryRun {
		log.Println("[DRY RUN MODE] - No notifications will be sent")
	}

	// Create GitHub client
	githubClient, err := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)
	if err != nil {
		log.Fatalf("Failed to create GitHub client: %v", err)
	}

	// Create Discord client
	var discordClient *discord.Client
	if cfg.DiscordWebhookURL != "" {
		discordClient = discord.NewClient(cfg.DiscordWebhookURL)
	}

	// Run daily update check
	report, err := tasks.CheckDailyUpdates(ctx, githubClient, discordClient, cfg)
	if err != nil {
		log.Fatalf("Daily update check failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("DAILY UPDATE CHECK REPORT")
	fmt.Println(strings.Repeat("=", 60))

	if cfg.DryRun {
		fmt.Println("[DRY RUN MODE - No notifications were sent]")
		fmt.Println()
	}

	fmt.Printf("Total issues checked: %d\n", report.TotalIssuesChecked)
	fmt.Printf("Stale issues found: %d\n", len(report.StaleIssues))

	if len(report.StaleIssues) > 0 {
		fmt.Println("\nStale issues by status:")
		byStatus := make(map[string]int)
		for _, stale := range report.StaleIssues {
			byStatus[stale.Issue.ProjectItem.StatusValue]++
		}
		for status, count := range byStatus {
			fmt.Printf("  - %s: %d\n", status, count)
		}
	}

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		for _, errMsg := range report.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))

	if cfg.DryRun {
		fmt.Println("\nThis was a dry run. Set DRY_RUN=false to send notifications.")
	} else if len(report.Errors) == 0 {
		log.Println("Daily update check completed successfully")
	}
}
