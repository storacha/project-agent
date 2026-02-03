package tasks

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
	"github.com/storacha/project-agent/internal/similarity"
)

// DuplicateGroup represents a group of potentially duplicate issues
type DuplicateGroup struct {
	Issues     []github.Issue
	Similarity float64
}

// DuplicateDetectionReport contains the results of duplicate detection
type DuplicateDetectionReport struct {
	IssuesAnalyzed  int
	DuplicateGroups []DuplicateGroup
	IssuesLabeled   int
	Errors          []string
}

// DetectDuplicates uses semantic similarity to find potential duplicate issues
func DetectDuplicates(ctx context.Context, githubClient *github.Client, similarityClient *similarity.Client, issues []github.Issue, cfg *config.Config) (*DuplicateDetectionReport, error) {
	report := &DuplicateDetectionReport{
		IssuesAnalyzed: len(issues),
	}

	log.Println("Detecting potential duplicate issues...")

	if len(issues) < 2 {
		return report, nil
	}

	var groups []DuplicateGroup
	processed := make(map[int]bool)

	for i, issue1 := range issues {
		if processed[issue1.Number] {
			continue
		}

		var group []github.Issue
		for j, issue2 := range issues {
			if i == j || processed[issue2.Number] {
				continue
			}

			similarityScore, err := similarityClient.CompareSimilarity(ctx, issue1, issue2)
			if err != nil {
				log.Printf("WARNING: Failed to compare issues #%d and #%d: %v\n",
					issue1.Number, issue2.Number, err)
				continue
			}

			if similarityScore >= cfg.DuplicateSimilarity {
				if len(group) == 0 {
					group = append(group, issue1)
					processed[issue1.Number] = true
				}
				group = append(group, issue2)
				processed[issue2.Number] = true
			}
		}

		if len(group) > 1 {
			groups = append(groups, DuplicateGroup{
				Issues:     group,
				Similarity: cfg.DuplicateSimilarity,
			})
		}
	}

	report.DuplicateGroups = groups
	log.Printf("Found %d potential duplicate groups\n", len(groups))

	// Label duplicate issues
	if len(groups) > 0 && !cfg.DryRun {
		for _, group := range groups {
			if err := labelDuplicates(ctx, githubClient, group); err != nil {
				errMsg := fmt.Sprintf("Failed to label duplicates: %v", err)
				log.Printf("WARNING: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				report.IssuesLabeled += len(group.Issues)
			}
			time.Sleep(2 * time.Second)
		}
	}

	return report, nil
}

// labelDuplicates adds a "possible duplicate" label to all issues in a duplicate group
func labelDuplicates(ctx context.Context, client *github.Client, group DuplicateGroup) error {
	for _, issue := range group.Issues {
		if err := client.AddLabel(ctx, issue, "possible duplicate"); err != nil {
			return fmt.Errorf("failed to label issue #%d: %w", issue.Number, err)
		}
		log.Printf("Added 'possible duplicate' label to issue #%d\n", issue.Number)
	}

	return nil
}
