package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shurcooL/githubv4"
	"github.com/storacha/project-agent/internal/config"
	"github.com/storacha/project-agent/internal/github"
	"github.com/storacha/project-agent/internal/similarity"
	"github.com/storacha/project-agent/internal/tasks"
	"golang.org/x/oauth2"
)

type Repository struct {
	Name         string
	Owner        struct {
		Login string
	}
	PullRequests struct {
		Nodes []struct {
			Number int
			Title  string
			Body   string
			State  string
		}
		PageInfo struct {
			EndCursor   githubv4.String
			HasNextPage bool
		}
	} `graphql:"pullRequests(first: 100, states: OPEN, after: $cursor)"`
}

type ScanReport struct {
	TotalRepos           int
	TotalPRsScanned      int
	TotalIssuesLinked    int
	TotalIssuesMoved     int
	ReposWithErrors      int
	Errors               []string
}

func main() {
	ctx := context.Background()

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Allow overriding to scan specific org
	org := os.Getenv("SCAN_ORG")
	if org == "" {
		org = cfg.GithubOrg
	}

	log.Println("Starting scan of open PRs across organization...")
	log.Printf("Organization: %s\n", org)
	log.Printf("Project: %d\n", cfg.ProjectNumber)
	if cfg.DryRun {
		log.Println("[DRY RUN MODE] - No changes will be made")
	}

	// Create GitHub client
	githubClient, err := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)
	if err != nil {
		log.Fatalf("Failed to create GitHub client: %v", err)
	}

	// Create similarity client
	similarityClient, err := similarity.NewClient(cfg.GeminiAPIKey)
	if err != nil {
		log.Fatalf("Failed to create similarity client: %v", err)
	}
	defer similarityClient.Close()

	// Create GraphQL client for repo/PR scanning
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.GithubToken},
	)
	httpClient := oauth2.NewClient(ctx, src)
	gqlClient := githubv4.NewClient(httpClient)

	// Fetch all repositories
	repos, err := fetchAllRepositories(ctx, gqlClient, org)
	if err != nil {
		log.Fatalf("Failed to fetch repositories: %v", err)
	}

	log.Printf("Found %d repositories\n", len(repos))

	// Scan each repository for open PRs
	scanReport := &ScanReport{
		TotalRepos: len(repos),
	}

	for _, repo := range repos {
		log.Printf("\n========================================\n")
		log.Printf("Scanning repository: %s/%s\n", repo.Owner.Login, repo.Name)
		log.Printf("========================================\n")

		// Fetch all open PRs for this repo
		prs, err := fetchOpenPRs(ctx, gqlClient, repo.Owner.Login, repo.Name)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch PRs for %s/%s: %v", repo.Owner.Login, repo.Name, err)
			log.Printf("ERROR: %s\n", errMsg)
			scanReport.Errors = append(scanReport.Errors, errMsg)
			scanReport.ReposWithErrors++
			continue
		}

		if len(prs) == 0 {
			log.Println("No open PRs found")
			continue
		}

		log.Printf("Found %d open PR(s)\n\n", len(prs))
		scanReport.TotalPRsScanned += len(prs)

		// Process each PR
		for _, pr := range prs {
			log.Printf("Processing PR #%d: %s\n", pr.Number, pr.Title)

			// Run PR linking
			report, err := tasks.LinkPRToIssues(ctx, githubClient, similarityClient,
				repo.Owner.Login, repo.Name, pr.Number, pr.Title, pr.Body, cfg)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to process PR %s/%s#%d: %v", repo.Owner.Login, repo.Name, pr.Number, err)
				log.Printf("ERROR: %s\n", errMsg)
				scanReport.Errors = append(scanReport.Errors, errMsg)
				continue
			}

			// Update scan report
			totalLinked := report.IssuesLinkedDirect + report.IssueLinkedSemantic
			scanReport.TotalIssuesLinked += totalLinked
			scanReport.TotalIssuesMoved += report.IssuesMovedToPRReview

			if len(report.Errors) > 0 {
				scanReport.Errors = append(scanReport.Errors, report.Errors...)
			}

			// Brief summary for this PR
			if totalLinked > 0 {
				log.Printf("  ✓ Linked to %d issue(s), moved %d to PR Review\n", totalLinked, report.IssuesMovedToPRReview)
			} else {
				log.Println("  - No issues linked")
			}

			// Rate limiting between PRs
			time.Sleep(2 * time.Second)
		}

		// Rate limiting between repos
		time.Sleep(3 * time.Second)
	}

	// Print final summary report
	printSummaryReport(scanReport, cfg.DryRun)
}

func fetchAllRepositories(ctx context.Context, client *githubv4.Client, org string) ([]Repository, error) {
	var query struct {
		Organization struct {
			Repositories struct {
				Nodes    []Repository
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"repositories(first: 100, after: $cursor)"`
		} `graphql:"organization(login: $org)"`
	}

	variables := map[string]interface{}{
		"org":    githubv4.String(org),
		"cursor": (*githubv4.String)(nil),
	}

	var allRepos []Repository

	for {
		if err := client.Query(ctx, &query, variables); err != nil {
			return nil, err
		}

		allRepos = append(allRepos, query.Organization.Repositories.Nodes...)

		if !query.Organization.Repositories.PageInfo.HasNextPage {
			break
		}

		variables["cursor"] = githubv4.NewString(query.Organization.Repositories.PageInfo.EndCursor)
	}

	return allRepos, nil
}

func fetchOpenPRs(ctx context.Context, client *githubv4.Client, owner, repo string) ([]struct {
	Number int
	Title  string
	Body   string
	State  string
}, error) {
	var query struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number int
					Title  string
					Body   string
					State  string
				}
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"pullRequests(first: 100, states: OPEN, after: $cursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"name":   githubv4.String(repo),
		"cursor": (*githubv4.String)(nil),
	}

	var allPRs []struct {
		Number int
		Title  string
		Body   string
		State  string
	}

	for {
		if err := client.Query(ctx, &query, variables); err != nil {
			return nil, err
		}

		allPRs = append(allPRs, query.Repository.PullRequests.Nodes...)

		if !query.Repository.PullRequests.PageInfo.HasNextPage {
			break
		}

		variables["cursor"] = githubv4.NewString(query.Repository.PullRequests.PageInfo.EndCursor)
	}

	return allPRs, nil
}

func printSummaryReport(report *ScanReport, dryRun bool) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("SCAN SUMMARY REPORT")
	fmt.Println(strings.Repeat("=", 60))

	if dryRun {
		fmt.Println("[DRY RUN MODE - No changes were made]")
		fmt.Println()
	}

	fmt.Printf("Repositories scanned: %d\n", report.TotalRepos)
	fmt.Printf("Total PRs processed: %d\n", report.TotalPRsScanned)
	fmt.Printf("Total issues linked: %d\n", report.TotalIssuesLinked)
	fmt.Printf("Total issues moved to PR Review: %d\n", report.TotalIssuesMoved)

	if report.ReposWithErrors > 0 {
		fmt.Printf("\nRepositories with errors: %d\n", report.ReposWithErrors)
	}

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors encountered: %d\n", len(report.Errors))
		fmt.Println("\nError details:")
		for i, errMsg := range report.Errors {
			if i < 10 { // Show first 10 errors
				fmt.Printf("  %d. %s\n", i+1, errMsg)
			}
		}
		if len(report.Errors) > 10 {
			fmt.Printf("  ... and %d more errors\n", len(report.Errors)-10)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))

	if dryRun {
		fmt.Println("\nThis was a dry run. Set DRY_RUN=false to apply changes.")
	} else {
		log.Println("Scan completed successfully")
	}
}
