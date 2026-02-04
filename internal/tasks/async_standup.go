package tasks

import (
	"context"
	"fmt"
	"log"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/discord"
)

// AsyncStandupReport contains the results of async standup thread creation
type AsyncStandupReport struct {
	ThreadCreated bool
	Error         string
}

// CreateAsyncStandup creates a new standup thread in Discord
func CreateAsyncStandup(ctx context.Context, discordClient *discord.Client, cfg *config.Config) (*AsyncStandupReport, error) {
	report := &AsyncStandupReport{}

	if cfg.DiscordStandupChannelID == "" {
		err := fmt.Errorf("DISCORD_STANDUP_CHANNEL_ID not configured")
		log.Printf("ERROR: %v\n", err)
		report.Error = err.Error()
		return report, err
	}

	log.Println("Creating async standup thread in Discord...")

	if cfg.DryRun {
		log.Println("[DRY RUN] Would create standup thread in channel:", cfg.DiscordStandupChannelID)
		if cfg.DiscordStandupRoleID != "" {
			log.Println("[DRY RUN] Would mention role:", cfg.DiscordStandupRoleID)
		}
		report.ThreadCreated = true
		return report, nil
	}

	// Create the standup thread
	err := discordClient.CreateStandupThread(ctx, cfg.DiscordStandupChannelID, cfg.DiscordStandupRoleID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create standup thread: %v", err)
		log.Printf("ERROR: %s\n", errMsg)
		report.Error = errMsg
		return report, err
	}

	report.ThreadCreated = true
	log.Println("Successfully created standup thread")

	return report, nil
}
