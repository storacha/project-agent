package tasks

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
	"github.com/storacha/project-agent/internal/parser"
	"github.com/storacha/project-agent/internal/similarity"
)

// PRLinkingReport contains the results of PR-to-issue linking
type PRLinkingReport struct {
	DirectReferencesFound int
	IssuesLinkedDirect    int
	SemanticMatchFound    bool
	IssueLinkedSemantic   int
	IssuesMovedToPRReview int
	Errors                []string
}

// LinkPRToIssues links a PR to related issues and moves them to PR Review status
func LinkPRToIssues(ctx context.Context, githubClient *github.Client, similarityClient *similarity.Client,
	prOwner, prRepo string, prNumber int, prTitle, prBody string, cfg *config.Config) (*PRLinkingReport, error) {

	report := &PRLinkingReport{}

	log.Printf("Processing PR %s/%s#%d\n", prOwner, prRepo, prNumber)

	// Step 1: Parse direct issue references from PR
	refs := parser.ParseIssueReferences(prTitle, prBody, prOwner, prRepo)
	report.DirectReferencesFound = len(refs)

	if len(refs) > 0 {
		log.Printf("Found %d direct issue reference(s)\n", len(refs))
		for _, ref := range refs {
			log.Printf("  - %s/%s#%d (explicit: %v)\n", ref.Owner, ref.Repo, ref.Number, ref.IsExplicit)
		}
	}

	// Step 2: For each referenced issue, check if it's in the project
	var matchedIssues []github.Issue
	for _, ref := range refs {
		issue, err := githubClient.GetIssueByNumber(ctx, ref.Owner, ref.Repo, ref.Number)
		if err != nil {
			log.Printf("WARNING: Issue %s/%s#%d not in project or not accessible: %v\n",
				ref.Owner, ref.Repo, ref.Number, err)
			continue
		}

		matchedIssues = append(matchedIssues, *issue)
		report.IssuesLinkedDirect++
	}

	log.Printf("Found %d referenced issue(s) in the project\n", len(matchedIssues))

	// Step 3: If no direct references, try semantic matching
	var semanticMatch *github.Issue
	if len(matchedIssues) == 0 {
		log.Println("No direct references found, attempting semantic matching...")

		// Fetch issues with target statuses (In Progress, Sprint Backlog)
		targetStatuses := []string{"In Progress", "Sprint Backlog"}
		issues, err := githubClient.GetIssuesByStatuses(ctx, targetStatuses)
		if err != nil {
			return report, fmt.Errorf("failed to fetch issues for semantic matching: %w", err)
		}

		log.Printf("Checking semantic similarity against %d issues\n", len(issues))

		if len(issues) > 0 {
			bestMatch, bestSimilarity, err := findBestSemanticMatch(ctx, similarityClient,
				prTitle, prBody, issues, cfg.DuplicateSimilarity)
			if err != nil {
				errMsg := fmt.Sprintf("Semantic matching failed: %v", err)
				log.Printf("WARNING: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else if bestMatch != nil {
				log.Printf("Found semantic match: issue #%d (similarity: %.2f)\n",
					bestMatch.Number, bestSimilarity)
				semanticMatch = bestMatch
				report.SemanticMatchFound = true
				report.IssueLinkedSemantic++
			} else {
				log.Println("No semantic matches found above threshold")
			}
		}
	}

	// Step 4: Move matched issues to PR Review and create links
	if !cfg.DryRun {
		// Handle direct references
		for _, issue := range matchedIssues {
			// Move to PR Review
			if err := githubClient.MoveToPRReview(ctx, issue); err != nil {
				errMsg := fmt.Sprintf("Failed to move issue #%d to PR Review: %v", issue.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				log.Printf("Moved issue #%d to PR Review status\n", issue.Number)
				report.IssuesMovedToPRReview++
			}

			// No need to create link - GitHub automatically links when PR references issue
			time.Sleep(1 * time.Second)
		}

		// Handle semantic match
		if semanticMatch != nil {
			// Move to PR Review
			if err := githubClient.MoveToPRReview(ctx, *semanticMatch); err != nil {
				errMsg := fmt.Sprintf("Failed to move issue #%d to PR Review: %v", semanticMatch.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				log.Printf("Moved issue #%d to PR Review status\n", semanticMatch.Number)
				report.IssuesMovedToPRReview++
			}

			// Create cross-reference link (adds minimal comment)
			if err := githubClient.LinkPRToIssue(ctx, prOwner, prRepo, prNumber, *semanticMatch); err != nil {
				errMsg := fmt.Sprintf("Failed to link PR to issue #%d: %v", semanticMatch.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				report.Errors = append(report.Errors, errMsg)
			} else {
				log.Printf("Created cross-reference link to issue #%d\n", semanticMatch.Number)
			}
		}
	} else {
		log.Println("[DRY RUN] Would move the following issues to PR Review:")
		for _, issue := range matchedIssues {
			log.Printf("  - Issue #%d (direct reference)\n", issue.Number)
		}
		if semanticMatch != nil {
			log.Printf("  - Issue #%d (semantic match)\n", semanticMatch.Number)
		}
	}

	return report, nil
}

// findBestSemanticMatch finds the most similar issue to the PR
func findBestSemanticMatch(ctx context.Context, client *similarity.Client,
	prTitle, prBody string, issues []github.Issue, threshold float64) (*github.Issue, float64, error) {

	var bestMatch *github.Issue
	var bestSimilarity float64

	// Create a pseudo-issue from the PR for comparison
	prIssue := github.Issue{
		Title: prTitle,
		Body:  prBody,
	}

	for _, issue := range issues {
		similarityScore, err := client.CompareSimilarity(ctx, prIssue, issue)
		if err != nil {
			log.Printf("WARNING: Failed to compare PR with issue #%d: %v\n", issue.Number, err)
			continue
		}

		if similarityScore > bestSimilarity && similarityScore >= threshold {
			bestSimilarity = similarityScore
			issueCopy := issue
			bestMatch = &issueCopy
		}

		// Rate limiting
		time.Sleep(200 * time.Millisecond)
	}

	return bestMatch, bestSimilarity, nil
}
