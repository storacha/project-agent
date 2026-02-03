package tasks

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
)

// StaleTriageReport contains the results of stale issue triage
type StaleTriageReport struct {
	IssuesAnalyzed   int
	StaleIssuesFound int
	IssuesMoved      int
	Errors           []string
}

// TriageStaleIssues identifies and moves stale issues to Stuck/Dead status
func TriageStaleIssues(ctx context.Context, client *github.Client, issues []github.Issue, cfg *config.Config) (*StaleTriageReport, error) {
	report := &StaleTriageReport{
		IssuesAnalyzed: len(issues),
	}

	// Identify stale issues
	log.Println("Analyzing issue staleness...")
	staleIssues := identifyStaleIssues(issues, cfg.StalenessThresholdDays)
	report.StaleIssuesFound = len(staleIssues)
	log.Printf("Found %d stale issues (>%d days)\n", len(staleIssues), cfg.StalenessThresholdDays)

	// Move stale issues to Stuck / Dead Issue status
	if len(staleIssues) > 0 {
		log.Println("Moving stale issues to Stuck / Dead Issue status...")
		for _, issue := range staleIssues {
			if cfg.DryRun {
				log.Printf("[DRY RUN] Would move issue #%d: %s\n", issue.Number, issue.Title)
				continue
			}

			err := moveStaleIssue(ctx, client, issue, cfg.StalenessThresholdDays)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to move issue #%d: %v", issue.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
				continue
			}

			report.IssuesMoved++
			log.Printf("Moved issue #%d to Stuck / Dead Issue\n", issue.Number)

			// Rate limit to avoid overwhelming GitHub API
			time.Sleep(2 * time.Second)
		}
	}

	return report, nil
}

// identifyStaleIssues finds issues that haven't been updated within the threshold
func identifyStaleIssues(issues []github.Issue, thresholdDays int) []github.Issue {
	threshold := time.Now().AddDate(0, 0, -thresholdDays)
	var staleIssues []github.Issue

	for _, issue := range issues {
		if issue.UpdatedAt.Before(threshold) {
			staleIssues = append(staleIssues, issue)
		}
	}

	return staleIssues
}

// moveStaleIssue moves an issue to Stuck / Dead Issue status and adds a comment
func moveStaleIssue(ctx context.Context, client *github.Client, issue github.Issue, thresholdDays int) error {
	// Add comment explaining why the issue is being moved
	daysSinceUpdate := int(time.Since(issue.UpdatedAt).Hours() / 24)
	comment := fmt.Sprintf(`This issue has been automatically moved to **Stuck / Dead Issue** status.

**Reason:** No activity for %d days (threshold: %d days)

If this issue is still relevant and you'd like to work on it, please:
1. Comment on this issue with an update
2. Move it back to Backlog or another appropriate status
3. Consider if this should be moved to Icebox instead

---
*Automated by project-agent*`, daysSinceUpdate, thresholdDays)

	if err := client.AddComment(ctx, issue, comment); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	// Move to Stuck / Dead Issue status
	if err := client.MoveToStuckDead(ctx, issue); err != nil {
		return fmt.Errorf("failed to move issue: %w", err)
	}

	return nil
}
