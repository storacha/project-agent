package github

import (
	"context"
	"fmt"
	"time"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// Client handles GitHub API interactions
type Client struct {
	client            *githubv4.Client
	org               string
	projectNumber     int
	projectID         string
	statusFieldID     string
	initiativeFieldID string
}

// Issue represents a GitHub issue with project metadata
type Issue struct {
	Number          int
	Title           string
	Body            string
	URL             string
	UpdatedAt       time.Time
	Assignees       []string // GitHub usernames
	ProjectItem     ProjectItemInfo
	RepositoryID    string
	RepositoryName  string
	RepositoryOwner string
}

// ProjectItemInfo contains project-specific metadata for an issue
type ProjectItemInfo struct {
	ID            string
	StatusValue   string
	StatusValueID string
	StatusFieldID string
}

// NewClient creates a new GitHub API client
func NewClient(token, org string, projectNumber int) (*Client, error) {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	client := githubv4.NewClient(httpClient)

	c := &Client{
		client:        client,
		org:           org,
		projectNumber: projectNumber,
	}

	// Fetch project metadata (ID and status field ID)
	if err := c.fetchProjectMetadata(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to fetch project metadata: %w", err)
	}

	return c, nil
}

// fetchProjectMetadata retrieves the project ID and status field ID
func (c *Client) fetchProjectMetadata(ctx context.Context) error {
	var query struct {
		Organization struct {
			ProjectV2 struct {
				ID     githubv4.ID
				Fields struct {
					Nodes []struct {
						TypeName    string `graphql:"__typename"`
						FieldCommon struct {
							ID   githubv4.ID
							Name githubv4.String
						} `graphql:"... on ProjectV2FieldCommon"`
						SingleSelectField struct {
							ID      githubv4.ID
							Name    githubv4.String
							Options []struct {
								ID   githubv4.String
								Name githubv4.String
							}
						} `graphql:"... on ProjectV2SingleSelectField"`
						TextField struct {
							ID   githubv4.ID
							Name githubv4.String
						} `graphql:"... on ProjectV2Field"`
					}
				} `graphql:"fields(first: 20)"`
			} `graphql:"projectV2(number: $projectNumber)"`
		} `graphql:"organization(login: $org)"`
	}

	variables := map[string]interface{}{
		"org":           githubv4.String(c.org),
		"projectNumber": githubv4.Int(c.projectNumber),
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return fmt.Errorf("failed to query project: %w", err)
	}

	projectID, ok := query.Organization.ProjectV2.ID.(string)
	if !ok {
		return fmt.Errorf("failed to convert project ID to string")
	}
	c.projectID = projectID

	// Find the Status and Initiative fields
	for _, field := range query.Organization.ProjectV2.Fields.Nodes {
		if field.TypeName == "ProjectV2SingleSelectField" {
			if string(field.SingleSelectField.Name) == "Status" {
				statusFieldID, ok := field.SingleSelectField.ID.(string)
				if !ok {
					return fmt.Errorf("failed to convert status field ID to string")
				}
				c.statusFieldID = statusFieldID
			}
		} else if field.TypeName == "ProjectV2Field" {
			if string(field.TextField.Name) == "Initiative" {
				initiativeFieldID, ok := field.TextField.ID.(string)
				if !ok {
					return fmt.Errorf("failed to convert initiative field ID to string")
				}
				c.initiativeFieldID = initiativeFieldID
			}
		}
	}

	if c.statusFieldID == "" {
		return fmt.Errorf("could not find Status field in project")
	}

	if c.initiativeFieldID == "" {
		return fmt.Errorf("could not find Initiative field in project")
	}

	return nil
}

// GetIssuesByStatuses retrieves all issues with any of the specified statuses
func (c *Client) GetIssuesByStatuses(ctx context.Context, statuses []string) ([]Issue, error) {
	statusMap := make(map[string]bool)
	for _, status := range statuses {
		statusMap[status] = true
	}

	return c.getFilteredIssues(ctx, statusMap)
}

// GetBacklogIssues retrieves all issues with Status = "Backlog"
// Deprecated: Use GetIssuesByStatuses instead
func (c *Client) GetBacklogIssues(ctx context.Context) ([]Issue, error) {
	return c.GetIssuesByStatuses(ctx, []string{"Backlog"})
}

// getFilteredIssues retrieves issues filtered by status
func (c *Client) getFilteredIssues(ctx context.Context, statusMap map[string]bool) ([]Issue, error) {
	var issues []Issue
	var cursor *githubv4.String

	for {
		var query struct {
			Node struct {
				ProjectV2 struct {
					Items struct {
						PageInfo struct {
							HasNextPage githubv4.Boolean
							EndCursor   githubv4.String
						}
						Nodes []struct {
							ID      githubv4.ID
							Content struct {
								TypeName string `graphql:"__typename"`
								Issue    struct {
									Number    githubv4.Int
									Title     githubv4.String
									Body      githubv4.String
									URL       githubv4.URI
									UpdatedAt githubv4.DateTime
									Assignees struct {
										Nodes []struct {
											Login githubv4.String
										}
									} `graphql:"assignees(first: 10)"`
									Repository struct {
										ID   githubv4.ID
										Name githubv4.String
									}
								} `graphql:"... on Issue"`
							}
							FieldValueByName struct {
								TypeName          string `graphql:"__typename"`
								SingleSelectValue struct {
									ID   githubv4.String
									Name githubv4.String
								} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
							} `graphql:"fieldValueByName(name: \"Status\")"`
						}
					} `graphql:"items(first: 100, after: $cursor)"`
				} `graphql:"... on ProjectV2"`
			} `graphql:"node(id: $projectID)"`
		}

		variables := map[string]interface{}{
			"projectID": githubv4.ID(c.projectID),
			"cursor":    cursor,
		}

		if err := c.client.Query(ctx, &query, variables); err != nil {
			return nil, fmt.Errorf("failed to query project items: %w", err)
		}

		for _, item := range query.Node.ProjectV2.Items.Nodes {
			// Only process issues (not PRs or draft issues)
			if item.Content.TypeName != "Issue" {
				continue
			}

			// Only process items with matching status
			statusName := string(item.FieldValueByName.SingleSelectValue.Name)
			if !statusMap[statusName] {
				continue
			}

			repoID, ok := item.Content.Issue.Repository.ID.(string)
			if !ok {
				continue // Skip if we can't get repo ID
			}

			itemID, ok := item.ID.(string)
			if !ok {
				continue // Skip if we can't get item ID
			}

			// Extract assignees
			assignees := []string{}
			for _, assignee := range item.Content.Issue.Assignees.Nodes {
				assignees = append(assignees, string(assignee.Login))
			}

			issues = append(issues, Issue{
				Number:         int(item.Content.Issue.Number),
				Title:          string(item.Content.Issue.Title),
				Body:           string(item.Content.Issue.Body),
				URL:            item.Content.Issue.URL.String(),
				UpdatedAt:      item.Content.Issue.UpdatedAt.Time,
				Assignees:      assignees,
				RepositoryID:   repoID,
				RepositoryName: string(item.Content.Issue.Repository.Name),
				ProjectItem: ProjectItemInfo{
					ID:            itemID,
					StatusValue:   statusName,
					StatusValueID: string(item.FieldValueByName.SingleSelectValue.ID),
					StatusFieldID: c.statusFieldID,
				},
			})
		}

		if !query.Node.ProjectV2.Items.PageInfo.HasNextPage {
			break
		}

		cursor = &query.Node.ProjectV2.Items.PageInfo.EndCursor
	}

	return issues, nil
}

// MoveToStuckDead moves an issue to "Stuck / Dead Issue" status
func (c *Client) MoveToStuckDead(ctx context.Context, issue Issue) error {
	// First, we need to get the option ID for "Stuck / Dead Issue" status
	stuckDeadOptionID, err := c.getStatusOptionID(ctx, "Stuck / Dead Issue")
	if err != nil {
		return fmt.Errorf("failed to get Stuck / Dead Issue option ID: %w", err)
	}

	var mutation struct {
		UpdateProjectV2ItemFieldValue struct {
			ProjectV2Item struct {
				ID githubv4.ID
			} `graphql:"projectV2Item"`
		} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
	}

	input := githubv4.UpdateProjectV2ItemFieldValueInput{
		ProjectID: githubv4.ID(c.projectID),
		ItemID:    githubv4.ID(issue.ProjectItem.ID),
		FieldID:   githubv4.ID(c.statusFieldID),
		Value: githubv4.ProjectV2FieldValue{
			SingleSelectOptionID: githubv4.NewString(githubv4.String(stuckDeadOptionID)),
		},
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return fmt.Errorf("failed to update project item: %w", err)
	}

	return nil
}

// getStatusOptionID retrieves the option ID for a given status name
func (c *Client) getStatusOptionID(ctx context.Context, statusName string) (string, error) {
	var query struct {
		Node struct {
			ProjectV2 struct {
				Field struct {
					TypeName          string `graphql:"__typename"`
					SingleSelectField struct {
						Options []struct {
							ID   githubv4.String
							Name githubv4.String
						}
					} `graphql:"... on ProjectV2SingleSelectField"`
				} `graphql:"field(name: \"Status\")"`
			} `graphql:"... on ProjectV2"`
		} `graphql:"node(id: $projectID)"`
	}

	variables := map[string]interface{}{
		"projectID": githubv4.ID(c.projectID),
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return "", fmt.Errorf("failed to query status options: %w", err)
	}

	for _, option := range query.Node.ProjectV2.Field.SingleSelectField.Options {
		if string(option.Name) == statusName {
			return string(option.ID), nil
		}
	}

	return "", fmt.Errorf("status option %q not found", statusName)
}

// AddLabel adds a label to an issue
func (c *Client) AddLabel(ctx context.Context, issue Issue, labelName string) error {
	// First, we need to get the label ID for the repository
	labelID, err := c.getLabelID(ctx, issue, labelName)
	if err != nil {
		// If label doesn't exist, create it first
		if err.Error() == fmt.Sprintf("label %q not found in repository", labelName) {
			createdLabelID, createErr := c.createLabel(ctx, issue, labelName)
			if createErr != nil {
				return fmt.Errorf("failed to create label: %w", createErr)
			}
			labelID = createdLabelID
		} else {
			return fmt.Errorf("failed to get label ID: %w", err)
		}
	}

	// Get the issue node ID
	issueNodeID, err := c.getIssueNodeID(ctx, issue)
	if err != nil {
		return fmt.Errorf("failed to get issue node ID: %w", err)
	}

	var mutation struct {
		AddLabelsToLabelable struct {
			Labelable struct {
				Labels struct {
					Nodes []struct {
						ID githubv4.ID
					}
				} `graphql:"labels(first: 10)"`
			}
		} `graphql:"addLabelsToLabelable(input: $input)"`
	}

	input := githubv4.AddLabelsToLabelableInput{
		LabelableID: issueNodeID,
		LabelIDs:    []githubv4.ID{labelID},
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return fmt.Errorf("failed to add label: %w", err)
	}

	return nil
}

// getLabelID retrieves the label ID for a given label name in the repository
func (c *Client) getLabelID(ctx context.Context, issue Issue, labelName string) (githubv4.ID, error) {
	var query struct {
		Node struct {
			Repository struct {
				Label struct {
					ID githubv4.ID
				} `graphql:"label(name: $labelName)"`
			} `graphql:"... on Repository"`
		} `graphql:"node(id: $repoID)"`
	}

	variables := map[string]interface{}{
		"repoID":    githubv4.ID(issue.RepositoryID),
		"labelName": githubv4.String(labelName),
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return "", fmt.Errorf("failed to query label: %w", err)
	}

	if query.Node.Repository.Label.ID == nil {
		return "", fmt.Errorf("label %q not found in repository", labelName)
	}

	return query.Node.Repository.Label.ID, nil
}

// createLabel creates a new label in the repository
func (c *Client) createLabel(ctx context.Context, issue Issue, labelName string) (githubv4.ID, error) {
	var mutation struct {
		CreateLabel struct {
			Label struct {
				ID githubv4.ID
			}
		} `graphql:"createLabel(input: $input)"`
	}

	// Define the input structure manually since githubv4 doesn't have CreateLabelInput
	input := map[string]interface{}{
		"repositoryId": githubv4.ID(issue.RepositoryID),
		"name":         githubv4.String(labelName),
		"color":        githubv4.String("d4c5f9"), // Light purple color
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return "", fmt.Errorf("failed to create label mutation: %w", err)
	}

	return mutation.CreateLabel.Label.ID, nil
}

// AddComment adds a comment to an issue using REST API via GraphQL
func (c *Client) AddComment(ctx context.Context, issue Issue, comment string) error {
	var mutation struct {
		AddComment struct {
			CommentEdge struct {
				Node struct {
					ID githubv4.ID
				}
			}
		} `graphql:"addComment(input: $input)"`
	}

	// We need to get the issue node ID first
	issueNodeID, err := c.getIssueNodeID(ctx, issue)
	if err != nil {
		return fmt.Errorf("failed to get issue node ID: %w", err)
	}

	input := githubv4.AddCommentInput{
		SubjectID: issueNodeID,
		Body:      githubv4.String(comment),
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	return nil
}

// MoveToPRReview moves an issue to "PR Review" status
func (c *Client) MoveToPRReview(ctx context.Context, issue Issue) error {
	// Get the option ID for "PR Review" status
	prReviewOptionID, err := c.getStatusOptionID(ctx, "PR Review")
	if err != nil {
		return fmt.Errorf("failed to get PR Review option ID: %w", err)
	}

	var mutation struct {
		UpdateProjectV2ItemFieldValue struct {
			ProjectV2Item struct {
				ID githubv4.ID
			} `graphql:"projectV2Item"`
		} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
	}

	input := githubv4.UpdateProjectV2ItemFieldValueInput{
		ProjectID: githubv4.ID(c.projectID),
		ItemID:    githubv4.ID(issue.ProjectItem.ID),
		FieldID:   githubv4.ID(c.statusFieldID),
		Value: githubv4.ProjectV2FieldValue{
			SingleSelectOptionID: githubv4.NewString(githubv4.String(prReviewOptionID)),
		},
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return fmt.Errorf("failed to update project item: %w", err)
	}

	return nil
}

// GetIssueByNumber retrieves an issue by repository and number, and checks if it's in the project
func (c *Client) GetIssueByNumber(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	// First, get the issue and repository ID
	var query struct {
		Repository struct {
			ID    githubv4.ID
			Issue struct {
				ID        githubv4.ID
				Number    githubv4.Int
				Title     githubv4.String
				Body      githubv4.String
				URL       githubv4.URI
				UpdatedAt githubv4.DateTime
			} `graphql:"issue(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"repo":   githubv4.String(repo),
		"number": githubv4.Int(number),
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return nil, fmt.Errorf("failed to query issue: %w", err)
	}

	repoID, ok := query.Repository.ID.(string)
	if !ok {
		return nil, fmt.Errorf("failed to convert repository ID")
	}

	issueNodeID := query.Repository.Issue.ID

	// Now check if this issue is in our project and get its project item info
	projectItem, err := c.getProjectItemForIssue(ctx, issueNodeID)
	if err != nil {
		return nil, fmt.Errorf("issue not in project: %w", err)
	}

	if projectItem == nil {
		return nil, fmt.Errorf("issue #%d not found in project", number)
	}

	return &Issue{
		Number:       int(query.Repository.Issue.Number),
		Title:        string(query.Repository.Issue.Title),
		Body:         string(query.Repository.Issue.Body),
		URL:          query.Repository.Issue.URL.String(),
		UpdatedAt:    query.Repository.Issue.UpdatedAt.Time,
		RepositoryID: repoID,
		ProjectItem:  *projectItem,
	}, nil
}

// getProjectItemForIssue finds the project item for a given issue node ID
func (c *Client) getProjectItemForIssue(ctx context.Context, issueNodeID githubv4.ID) (*ProjectItemInfo, error) {
	var query struct {
		Node struct {
			Issue struct {
				ProjectItems struct {
					Nodes []struct {
						ID      githubv4.ID
						Project struct {
							ID githubv4.ID
						}
						FieldValueByName struct {
							TypeName          string `graphql:"__typename"`
							SingleSelectValue struct {
								ID   githubv4.String
								Name githubv4.String
							} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
						} `graphql:"fieldValueByName(name: \"Status\")"`
					}
				} `graphql:"projectItems(first: 10)"`
			} `graphql:"... on Issue"`
		} `graphql:"node(id: $issueID)"`
	}

	variables := map[string]interface{}{
		"issueID": issueNodeID,
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return nil, fmt.Errorf("failed to query project items: %w", err)
	}

	// Find the item that belongs to our project
	for _, item := range query.Node.Issue.ProjectItems.Nodes {
		projectID, ok := item.Project.ID.(string)
		if !ok {
			continue
		}

		if projectID == c.projectID {
			itemID, ok := item.ID.(string)
			if !ok {
				continue
			}

			return &ProjectItemInfo{
				ID:            itemID,
				StatusValue:   string(item.FieldValueByName.SingleSelectValue.Name),
				StatusValueID: string(item.FieldValueByName.SingleSelectValue.ID),
				StatusFieldID: c.statusFieldID,
			}, nil
		}
	}

	return nil, nil
}

// LinkPRToIssue creates a cross-reference between a PR and an issue
// This makes the PR appear in the issue's timeline
func (c *Client) LinkPRToIssue(ctx context.Context, prOwner, prRepo string, prNumber int, issue Issue) error {
	// Get the issue node ID
	issueNodeID, err := c.getIssueNodeID(ctx, issue)
	if err != nil {
		return fmt.Errorf("failed to get issue node ID: %w", err)
	}

	// Create a comment on the issue that references the PR
	// This creates a cross-reference link that shows in the timeline
	prRef := fmt.Sprintf("%s/%s#%d", prOwner, prRepo, prNumber)
	comment := fmt.Sprintf("Linked to PR %s", prRef)

	var mutation struct {
		AddComment struct {
			CommentEdge struct {
				Node struct {
					ID githubv4.ID
				}
			}
		} `graphql:"addComment(input: $input)"`
	}

	input := githubv4.AddCommentInput{
		SubjectID: issueNodeID,
		Body:      githubv4.String(comment),
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return fmt.Errorf("failed to create cross-reference: %w", err)
	}

	return nil
}

// getIssueNodeID retrieves the global node ID for an issue
func (c *Client) getIssueNodeID(ctx context.Context, issue Issue) (githubv4.ID, error) {
	var query struct {
		Node struct {
			Repository struct {
				Issue struct {
					ID githubv4.ID
				} `graphql:"issue(number: $number)"`
			} `graphql:"... on Repository"`
		} `graphql:"node(id: $repoID)"`
	}

	variables := map[string]interface{}{
		"repoID": githubv4.ID(issue.RepositoryID),
		"number": githubv4.Int(issue.Number),
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return "", fmt.Errorf("failed to query issue: %w", err)
	}

	return query.Node.Repository.Issue.ID, nil
}

// GetInitiativeIssues retrieves all issues with GitHub Issue Type = "Initiative"
func (c *Client) GetInitiativeIssues(ctx context.Context) ([]Issue, error) {
	var issues []Issue
	var cursor *githubv4.String

	for {
		var query struct {
			Node struct {
				ProjectV2 struct {
					Items struct {
						PageInfo struct {
							HasNextPage githubv4.Boolean
							EndCursor   githubv4.String
						}
						Nodes []struct {
							ID      githubv4.ID
							Content struct {
								TypeName string `graphql:"__typename"`
								Issue    struct {
									Number    githubv4.Int
									Title     githubv4.String
									Body      githubv4.String
									URL       githubv4.URI
									UpdatedAt githubv4.DateTime
									IssueType struct {
										Name githubv4.String
									}
									Assignees struct {
										Nodes []struct {
											Login githubv4.String
										}
									} `graphql:"assignees(first: 10)"`
									Repository struct {
										ID    githubv4.ID
										Name  githubv4.String
										Owner struct {
											Login githubv4.String
										}
									}
								} `graphql:"... on Issue"`
							}
							StatusField struct {
								TypeName          string `graphql:"__typename"`
								SingleSelectValue struct {
									ID   githubv4.String
									Name githubv4.String
								} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
							} `graphql:"statusField: fieldValueByName(name: \"Status\")"`
						}
					} `graphql:"items(first: 100, after: $cursor)"`
				} `graphql:"... on ProjectV2"`
			} `graphql:"node(id: $projectID)"`
		}

		variables := map[string]interface{}{
			"projectID": githubv4.ID(c.projectID),
			"cursor":    cursor,
		}

		if err := c.client.Query(ctx, &query, variables); err != nil {
			return nil, fmt.Errorf("failed to query project items: %w", err)
		}

		for _, item := range query.Node.ProjectV2.Items.Nodes {
			// Only process issues (not PRs or draft issues)
			if item.Content.TypeName != "Issue" {
				continue
			}

			// Only process items with GitHub Issue Type = "Initiative"
			issueTypeName := string(item.Content.Issue.IssueType.Name)
			if issueTypeName != "Initiative" {
				continue
			}

			repoID, ok := item.Content.Issue.Repository.ID.(string)
			if !ok {
				continue // Skip if we can't get repo ID
			}

			itemID, ok := item.ID.(string)
			if !ok {
				continue // Skip if we can't get item ID
			}

			// Extract assignees
			assignees := []string{}
			for _, assignee := range item.Content.Issue.Assignees.Nodes {
				assignees = append(assignees, string(assignee.Login))
			}

			statusName := string(item.StatusField.SingleSelectValue.Name)

			issues = append(issues, Issue{
				Number:          int(item.Content.Issue.Number),
				Title:           string(item.Content.Issue.Title),
				Body:            string(item.Content.Issue.Body),
				URL:             item.Content.Issue.URL.String(),
				UpdatedAt:       item.Content.Issue.UpdatedAt.Time,
				Assignees:       assignees,
				RepositoryID:    repoID,
				RepositoryName:  string(item.Content.Issue.Repository.Name),
				RepositoryOwner: string(item.Content.Issue.Repository.Owner.Login),
				ProjectItem: ProjectItemInfo{
					ID:            itemID,
					StatusValue:   statusName,
					StatusValueID: string(item.StatusField.SingleSelectValue.ID),
					StatusFieldID: c.statusFieldID,
				},
			})
		}

		if !query.Node.ProjectV2.Items.PageInfo.HasNextPage {
			break
		}

		cursor = &query.Node.ProjectV2.Items.PageInfo.EndCursor
	}

	return issues, nil
}

// SubIssue represents a sub-issue with owner, repo, and number
type SubIssue struct {
	Owner  string
	Repo   string
	Number int
	Title  string
}

// GetSubIssuesRecursive fetches all sub-issues (and descendants) for a given issue
func (c *Client) GetSubIssuesRecursive(ctx context.Context, owner, repo string, number int) ([]SubIssue, error) {
	var allSubIssues []SubIssue
	visited := make(map[string]bool)

	var fetchSubIssues func(string, string, int) error
	fetchSubIssues = func(owner, repo string, number int) error {
		key := fmt.Sprintf("%s/%s#%d", owner, repo, number)
		if visited[key] {
			return nil // Avoid infinite loops
		}
		visited[key] = true

		var query struct {
			Repository struct {
				Issue struct {
					SubIssues struct {
						Nodes []struct {
							Number githubv4.Int
							Title  githubv4.String
							Repository struct {
								Name  githubv4.String
								Owner struct {
									Login githubv4.String
								}
							}
						}
					} `graphql:"subIssues(first: 100)"`
				} `graphql:"issue(number: $number)"`
			} `graphql:"repository(owner: $owner, name: $repo)"`
		}

		variables := map[string]interface{}{
			"owner":  githubv4.String(owner),
			"repo":   githubv4.String(repo),
			"number": githubv4.Int(number),
		}

		if err := c.client.Query(ctx, &query, variables); err != nil {
			return fmt.Errorf("failed to query sub-issues for %s/%s#%d: %w", owner, repo, number, err)
		}

		for _, node := range query.Repository.Issue.SubIssues.Nodes {
			subIssue := SubIssue{
				Owner:  string(node.Repository.Owner.Login),
				Repo:   string(node.Repository.Name),
				Number: int(node.Number),
				Title:  string(node.Title),
			}
			allSubIssues = append(allSubIssues, subIssue)

			// Recursively fetch sub-issues of this sub-issue
			if err := fetchSubIssues(subIssue.Owner, subIssue.Repo, subIssue.Number); err != nil {
				return err
			}
		}

		return nil
	}

	if err := fetchSubIssues(owner, repo, number); err != nil {
		return nil, err
	}

	return allSubIssues, nil
}

// AddIssueToProject adds an issue to the project with "Inbox" status
func (c *Client) AddIssueToProject(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	// First, get the issue and repository IDs
	var query struct {
		Repository struct {
			ID    githubv4.ID
			Issue struct {
				ID        githubv4.ID
				Number    githubv4.Int
				Title     githubv4.String
				Body      githubv4.String
				URL       githubv4.URI
				UpdatedAt githubv4.DateTime
			} `graphql:"issue(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"repo":   githubv4.String(repo),
		"number": githubv4.Int(number),
	}

	if err := c.client.Query(ctx, &query, variables); err != nil {
		return nil, fmt.Errorf("failed to query issue: %w", err)
	}

	repoID, ok := query.Repository.ID.(string)
	if !ok {
		return nil, fmt.Errorf("failed to convert repository ID")
	}

	issueNodeID := query.Repository.Issue.ID

	// Check if issue is already in the project
	existingItem, err := c.getProjectItemForIssue(ctx, issueNodeID)
	if err == nil && existingItem != nil {
		// Issue is already in project, return it
		return &Issue{
			Number:       int(query.Repository.Issue.Number),
			Title:        string(query.Repository.Issue.Title),
			Body:         string(query.Repository.Issue.Body),
			URL:          query.Repository.Issue.URL.String(),
			UpdatedAt:    query.Repository.Issue.UpdatedAt.Time,
			RepositoryID: repoID,
			ProjectItem:  *existingItem,
		}, nil
	}

	// Add the issue to the project
	var addMutation struct {
		AddProjectV2ItemById struct {
			Item struct {
				ID githubv4.ID
			}
		} `graphql:"addProjectV2ItemById(input: $input)"`
	}

	addInput := githubv4.AddProjectV2ItemByIdInput{
		ProjectID: githubv4.ID(c.projectID),
		ContentID: issueNodeID,
	}

	if err := c.client.Mutate(ctx, &addMutation, addInput, nil); err != nil {
		return nil, fmt.Errorf("failed to add issue to project: %w", err)
	}

	itemID, ok := addMutation.AddProjectV2ItemById.Item.ID.(string)
	if !ok {
		return nil, fmt.Errorf("failed to convert item ID")
	}

	// Set status to "Inbox"
	inboxOptionID, err := c.getStatusOptionID(ctx, "Inbox")
	if err != nil {
		return nil, fmt.Errorf("failed to get Inbox option ID: %w", err)
	}

	var updateMutation struct {
		UpdateProjectV2ItemFieldValue struct {
			ProjectV2Item struct {
				ID githubv4.ID
			} `graphql:"projectV2Item"`
		} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
	}

	updateInput := githubv4.UpdateProjectV2ItemFieldValueInput{
		ProjectID: githubv4.ID(c.projectID),
		ItemID:    githubv4.ID(itemID),
		FieldID:   githubv4.ID(c.statusFieldID),
		Value: githubv4.ProjectV2FieldValue{
			SingleSelectOptionID: githubv4.NewString(githubv4.String(inboxOptionID)),
		},
	}

	if err := c.client.Mutate(ctx, &updateMutation, updateInput, nil); err != nil {
		return nil, fmt.Errorf("failed to set status to Inbox: %w", err)
	}

	return &Issue{
		Number:       int(query.Repository.Issue.Number),
		Title:        string(query.Repository.Issue.Title),
		Body:         string(query.Repository.Issue.Body),
		URL:          query.Repository.Issue.URL.String(),
		UpdatedAt:    query.Repository.Issue.UpdatedAt.Time,
		RepositoryID: repoID,
		ProjectItem: ProjectItemInfo{
			ID:            itemID,
			StatusValue:   "Inbox",
			StatusValueID: inboxOptionID,
			StatusFieldID: c.statusFieldID,
		},
	}, nil
}

// UpdateInitiativeField sets the Initiative text field for a project item
func (c *Client) UpdateInitiativeField(ctx context.Context, issue Issue, initiativeTitle string) error {
	var mutation struct {
		UpdateProjectV2ItemFieldValue struct {
			ProjectV2Item struct {
				ID githubv4.ID
			} `graphql:"projectV2Item"`
		} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
	}

	input := githubv4.UpdateProjectV2ItemFieldValueInput{
		ProjectID: githubv4.ID(c.projectID),
		ItemID:    githubv4.ID(issue.ProjectItem.ID),
		FieldID:   githubv4.ID(c.initiativeFieldID),
		Value: githubv4.ProjectV2FieldValue{
			Text: githubv4.NewString(githubv4.String(initiativeTitle)),
		},
	}

	if err := c.client.Mutate(ctx, &mutation, input, nil); err != nil {
		return fmt.Errorf("failed to update Initiative field: %w", err)
	}

	return nil
}
