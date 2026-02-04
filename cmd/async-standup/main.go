package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/discord"
	"github.com/storacha/project-agent/internal/tasks"
)

func main() {
	ctx := context.Background()

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate Discord bot token
	if cfg.DiscordBotToken == "" {
		log.Fatalf("DISCORD_BOT_TOKEN is required for async standup")
	}

	// Create Discord bot client
	discordClient := discord.NewBotClient(cfg.DiscordBotToken)

	log.Println("Starting async standup thread creation...")
	if cfg.DryRun {
		log.Println("DRY RUN MODE: No thread will be created")
	}

	// Create the standup thread
	report, err := tasks.CreateAsyncStandup(ctx, discordClient, cfg)
	if err != nil {
		log.Fatalf("Async standup failed: %v", err)
	}

	// Print summary report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("ASYNC STANDUP REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Run Date: %s\n\n", time.Now().Format(time.RFC3339))

	if report.ThreadCreated {
		fmt.Println("✅ Standup thread created successfully")
	} else {
		fmt.Println("❌ Failed to create standup thread")
	}

	if report.Error != "" {
		fmt.Printf("\nError: %s\n", report.Error)
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	log.Println("Async standup completed successfully")
}
