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

	log.Println("Starting weekly DM distribution...")
	log.Printf("Organization: %s\n", cfg.GithubOrg)
	log.Printf("Project: %d\n", cfg.ProjectNumber)
	log.Printf("Users to notify: %d\n", len(cfg.UserMappings))
	if cfg.DryRun {
		log.Println("[DRY RUN MODE] - No DMs will be sent")
	}

	// Validate Discord bot token
	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable is required")
	}

	if len(cfg.UserMappings) == 0 {
		log.Fatal("USER_MAPPINGS is empty - no users to notify")
	}

	// Create GitHub client
	githubClient, err := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)
	if err != nil {
		log.Fatalf("Failed to create GitHub client: %v", err)
	}

	// Create Discord bot client
	discordClient := discord.NewBotClient(cfg.DiscordBotToken)

	// Send weekly DMs
	report, err := tasks.SendWeeklyDMs(ctx, githubClient, discordClient, cfg)
	if err != nil {
		log.Fatalf("Weekly DM distribution failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("WEEKLY DM REPORT")
	fmt.Println(strings.Repeat("=", 60))

	if cfg.DryRun {
		fmt.Println("[DRY RUN MODE - No DMs were sent]")
		fmt.Println()
	}

	fmt.Printf("Total users in mappings: %d\n", report.TotalUsers)
	fmt.Printf("Total active issues: %d\n", report.TotalIssues)
	fmt.Printf("Unassigned issues: %d\n", report.UnassignedIssuesCount)
	fmt.Printf("User DMs sent: %d\n", report.DMsSent)
	fmt.Printf("Users with no assigned issues: %d\n", report.UsersWithNoIssues)

	if report.UnassignedIssuesDMSent {
		fmt.Println("\n✓ Unassigned issues report sent")
	} else if cfg.UnassignedIssuesUserID == "" {
		fmt.Println("\n- Unassigned issues report skipped (UNASSIGNED_ISSUES_USER_ID not set)")
	}

	if report.UsersNotInMappings > 0 {
		fmt.Printf("\nUsers not in mappings (skipped): %d\n", report.UsersNotInMappings)
	}

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		for _, errMsg := range report.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))

	if cfg.DryRun {
		fmt.Println("\nThis was a dry run. Set DRY_RUN=false to send DMs.")
	} else if len(report.Errors) == 0 {
		log.Println("Weekly DM distribution completed successfully")
	}
}
