package tasks

import (
	"context"
	"log"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/discord"
	"github.com/storacha/project-agent/internal/github"
)

// DailyUpdateReport contains the results of the daily update check
type DailyUpdateReport struct {
	TotalIssuesChecked int
	StaleIssues        []discord.StaleIssue
	Errors             []string
}

// CheckDailyUpdates checks active issues for staleness and reports to Discord
func CheckDailyUpdates(ctx context.Context, githubClient *github.Client, discordClient *discord.Client, cfg *config.Config) (*DailyUpdateReport, error) {
	report := &DailyUpdateReport{}

	// Fetch issues with active statuses (Sprint Backlog, In Progress, PR Review)
	activeStatuses := []string{"Sprint Backlog", "In Progress", "PR Review"}
	log.Printf("Fetching issues with statuses: %v\n", activeStatuses)

	issues, err := githubClient.GetIssuesByStatuses(ctx, activeStatuses)
	if err != nil {
		return report, err
	}

	report.TotalIssuesChecked = len(issues)
	log.Printf("Found %d active issues to check\n", len(issues))

	// Check each issue for staleness
	now := time.Now()
	threshold := time.Duration(cfg.DailyUpdateThreshold) * 24 * time.Hour

	for _, issue := range issues {
		daysSinceUpdate := int(now.Sub(issue.UpdatedAt).Hours() / 24)

		if now.Sub(issue.UpdatedAt) > threshold {
			log.Printf("Issue #%d is stale (%d days since update)\n", issue.Number, daysSinceUpdate)

			staleIssue := discord.StaleIssue{
				Issue:           issue,
				DaysSinceUpdate: daysSinceUpdate,
				AssignedTo:      issue.Assignees,
			}

			report.StaleIssues = append(report.StaleIssues, staleIssue)
		}
	}

	log.Printf("Found %d stale issues\n", len(report.StaleIssues))

	// Send Discord notification
	if !cfg.DryRun {
		if cfg.DiscordWebhookURL == "" {
			log.Println("WARNING: DISCORD_WEBHOOK_URL not set, skipping Discord notification")
		} else {
			log.Println("Sending Discord notification...")
			if err := discordClient.SendStaleIssuesReport(ctx, report.StaleIssues, cfg.UserMappings); err != nil {
				errMsg := "Failed to send Discord notification: " + err.Error()
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				log.Println("Discord notification sent successfully")
			}
		}
	} else {
		log.Println("[DRY RUN] Would send Discord notification for the following stale issues:")
		for _, stale := range report.StaleIssues {
			assignees := "unassigned"
			if len(stale.AssignedTo) > 0 {
				assignees = ""
				for i, a := range stale.AssignedTo {
					if i > 0 {
						assignees += ", "
					}
					assignees += a
				}
			}
			log.Printf("  -  [%s #%d]: %s (%d days, assigned: %s)\n",
				stale.Issue.RepositoryName, stale.Issue.Number, stale.Issue.Title, stale.DaysSinceUpdate, assignees)
		}
	}

	return report, nil
}
