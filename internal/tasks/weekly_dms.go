package tasks

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/discord"
	"github.com/storacha/project-agent/internal/github"
)

// WeeklyDMReport contains the results of sending weekly DMs
type WeeklyDMReport struct {
	TotalUsers              int
	TotalIssues             int
	UnassignedIssuesCount   int
	DMsSent                 int
	UsersWithNoIssues       int
	UsersNotInMappings      int
	UnassignedIssuesDMSent  bool
	Errors                  []string
}

// SendWeeklyDMs sends DMs to each team member with their assigned issues
func SendWeeklyDMs(ctx context.Context, githubClient *github.Client, discordClient *discord.Client, cfg *config.Config) (*WeeklyDMReport, error) {
	report := &WeeklyDMReport{}

	log.Println("Fetching issues from active statuses...")

	// Fetch issues with active statuses (Sprint Backlog, In Progress, PR Review)
	activeStatuses := []string{"Sprint Backlog", "In Progress", "PR Review"}
	issues, err := githubClient.GetIssuesByStatuses(ctx, activeStatuses)
	if err != nil {
		return report, fmt.Errorf("failed to fetch issues: %w", err)
	}

	log.Printf("Found %d active issues\n", len(issues))
	report.TotalIssues = len(issues)

	// Group issues by assignee and collect unassigned
	issuesByUser := make(map[string][]github.Issue)
	var unassignedIssues []github.Issue

	for _, issue := range issues {
		if len(issue.Assignees) == 0 {
			unassignedIssues = append(unassignedIssues, issue)
			continue
		}

		for _, assignee := range issue.Assignees {
			issuesByUser[assignee] = append(issuesByUser[assignee], issue)
		}
	}

	report.UnassignedIssuesCount = len(unassignedIssues)
	log.Printf("Found %d unassigned issues\n", len(unassignedIssues))

	log.Printf("Issues are assigned to %d unique users\n", len(issuesByUser))

	// Get all users from mappings (these are the users we want to DM)
	usersToNotify := make(map[string]bool)
	for githubUser := range cfg.UserMappings {
		usersToNotify[githubUser] = true
	}

	report.TotalUsers = len(usersToNotify)
	log.Printf("Will send DMs to %d users from mappings\n", report.TotalUsers)

	// Send DM to each user
	for githubUser := range usersToNotify {
		discordUserID, ok := cfg.UserMappings[githubUser]
		if !ok {
			// This shouldn't happen since we're iterating over the mappings, but just in case
			report.UsersNotInMappings++
			continue
		}

		userIssues := issuesByUser[githubUser]

		if len(userIssues) == 0 {
			log.Printf("User %s has no assigned issues, skipping DM\n", githubUser)
			report.UsersWithNoIssues++
			continue
		}

		log.Printf("Sending DM to %s (%d issues)...\n", githubUser, len(userIssues))

		if !cfg.DryRun {
			userIssuesData := discord.UserIssues{
				GithubUsername: githubUser,
				DiscordUserID:  discordUserID,
				Issues:         userIssues,
			}

			if err := discordClient.SendWeeklyDM(ctx, userIssuesData); err != nil {
				errMsg := fmt.Sprintf("Failed to send DM to %s: %v", githubUser, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				log.Printf("Successfully sent DM to %s\n", githubUser)
				report.DMsSent++
			}

			// Rate limiting - be nice to Discord API
			time.Sleep(1 * time.Second)
		} else {
			log.Printf("[DRY RUN] Would send DM to %s with %d issues\n", githubUser, len(userIssues))
			for _, issue := range userIssues {
				log.Printf("  - #%d [%s]: %s\n", issue.Number, issue.ProjectItem.StatusValue, issue.Title)
			}
			report.DMsSent++
		}
	}

	// Send unassigned issues DM if configured
	if cfg.UnassignedIssuesUserID != "" {
		log.Printf("\nSending unassigned issues report to designated user...\n")

		if !cfg.DryRun {
			if err := discordClient.SendUnassignedIssuesDM(ctx, cfg.UnassignedIssuesUserID, unassignedIssues); err != nil {
				errMsg := fmt.Sprintf("Failed to send unassigned issues DM: %v", err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				log.Printf("Successfully sent unassigned issues DM (%d issues)\n", len(unassignedIssues))
				report.UnassignedIssuesDMSent = true
			}
		} else {
			log.Printf("[DRY RUN] Would send unassigned issues DM with %d issues:\n", len(unassignedIssues))
			for _, issue := range unassignedIssues {
				log.Printf("  - #%d [%s]: %s\n", issue.Number, issue.ProjectItem.StatusValue, issue.Title)
			}
			report.UnassignedIssuesDMSent = true
		}
	} else {
		log.Println("\nUNASSIGNED_ISSUES_USER_ID not set, skipping unassigned issues report")
	}

	return report, nil
}
