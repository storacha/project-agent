package tasks

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
)

// ProcessInitiativesReport contains the results of initiative processing
type ProcessInitiativesReport struct {
	InitiativesProcessed int
	SubIssuesFound       int
	SubIssuesAdded       int
	SubIssuesUpdated     int
	Errors               []string
}

// ProcessInitiatives finds all Initiative-type issues and processes their sub-issues
func ProcessInitiatives(ctx context.Context, client *github.Client, initiatives []github.Issue, cfg *config.Config) (*ProcessInitiativesReport, error) {
	report := &ProcessInitiativesReport{
		InitiativesProcessed: len(initiatives),
	}

	log.Printf("Processing %d initiatives...\n", len(initiatives))

	for _, initiative := range initiatives {
		log.Printf("Processing initiative #%d: %s\n", initiative.Number, initiative.Title)

		// Get all sub-issues recursively
		subIssues, err := client.GetSubIssuesRecursive(ctx, initiative.RepositoryOwner, initiative.RepositoryName, initiative.Number)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch sub-issues for initiative #%d: %v", initiative.Number, err)
			log.Printf("ERROR: %s\n", errMsg)
			report.Errors = append(report.Errors, errMsg)
			continue
		}

		log.Printf("Found %d sub-issues (including descendants) for initiative #%d\n", len(subIssues), initiative.Number)
		report.SubIssuesFound += len(subIssues)

		// Process each sub-issue
		for _, subIssue := range subIssues {
			if cfg.DryRun {
				log.Printf("[DRY RUN] Would process sub-issue %s/%s#%d: %s\n",
					subIssue.Owner, subIssue.Repo, subIssue.Number, subIssue.Title)
				log.Printf("[DRY RUN]   - Add to project (if not present) with status 'Inbox'\n")
				log.Printf("[DRY RUN]   - Set Initiative field to '%s'\n", initiative.Title)
				continue
			}

			// Add sub-issue to project (or get existing)
			issue, err := client.AddIssueToProject(ctx, subIssue.Owner, subIssue.Repo, subIssue.Number)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to add sub-issue %s/%s#%d to project: %v",
					subIssue.Owner, subIssue.Repo, subIssue.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
				continue
			}

			// Check if issue was just added (status is "Inbox") or already existed
			if issue.ProjectItem.StatusValue == "Inbox" {
				report.SubIssuesAdded++
				log.Printf("Added sub-issue %s/%s#%d to project with status 'Inbox'\n",
					subIssue.Owner, subIssue.Repo, subIssue.Number)
			}

			// Update Initiative field
			err = client.UpdateInitiativeField(ctx, *issue, initiative.Title)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to update Initiative field for %s/%s#%d: %v",
					subIssue.Owner, subIssue.Repo, subIssue.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
				continue
			}

			report.SubIssuesUpdated++
			log.Printf("Set Initiative field to '%s' for %s/%s#%d\n",
				initiative.Title, subIssue.Owner, subIssue.Repo, subIssue.Number)

			// Rate limit to avoid overwhelming GitHub API
			time.Sleep(2 * time.Second)
		}
	}

	return report, nil
}
