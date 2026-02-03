# Project Agent Setup Guide

This guide walks you through setting up the project agent to automatically maintain your GitHub Project backlog.

## Prerequisites

1. **GitHub Account** with admin access to the storacha organization
2. **Google Gemini API Key** - Get one at https://ai.google.dev/
3. **Go 1.22+** (for local testing, optional)

## Step 1: Create GitHub Personal Access Token

1. Go to https://github.com/settings/tokens?type=beta
2. Click "Generate new token" (use Fine-grained tokens)
3. Configure the token:
   - **Token name**: `project-maintenance-bot`
   - **Expiration**: 90 days (or custom)
   - **Repository access**: All repositories (or select specific ones)
   - **Organization permissions**:
     - Projects: Read and write
   - **Repository permissions**:
     - Issues: Read and write
     - Contents: Read (for the project-agent repo)

4. Click "Generate token" and **save the token securely**

## Step 2: Get Gemini API Key

1. Go to https://ai.google.dev/
2. Click "Get API key in Google AI Studio"
3. Create a new API key
4. **Save the API key securely**

## Step 3: Push Code to GitHub

```bash
cd /Users/hannah/projects/go/src/github.com/storacha/project-agent

# Initialize git repository
git init

# Add all files
git add .

# Create initial commit
git commit -m "Initial commit: Project backlog maintenance agent"

# Create repository on GitHub first, then:
git remote add origin git@github.com:storacha/project-agent.git
git branch -M main
git push -u origin main
```

## Step 4: Configure GitHub Secrets

1. Go to https://github.com/storacha/project-agent/settings/secrets/actions
2. Click "New repository secret"
3. Add the following secrets:

   **Secret 1:**
   - Name: `PROJECT_MAINTENANCE_TOKEN`
   - Value: Your GitHub PAT from Step 1

   **Secret 2:**
   - Name: `GEMINI_API_KEY`
   - Value: Your Gemini API key from Step 2

## Step 5: Test Locally (Optional but Recommended)

Before running in production, test locally in dry-run mode:

```bash
# Copy the example env file
cp .env.example .env

# Edit .env with your credentials
nano .env

# Run in dry-run mode (no actual changes)
export DRY_RUN=true
go run main.go
```

This will show you what the agent would do without making any actual changes.

## Step 6: Enable GitHub Actions

1. Go to https://github.com/storacha/project-agent/actions
2. If Actions are disabled, click "I understand my workflows, go ahead and enable them"
3. You should see "Daily Backlog Maintenance" workflow

## Step 7: Test the Workflow

1. Go to the Actions tab
2. Click "Daily Backlog Maintenance"
3. Click "Run workflow" dropdown
4. Select the `main` branch
5. Click "Run workflow"

The workflow will run and you can see the results in real-time.

## Step 8: Verify the Results

After the workflow completes:

1. Check the workflow run logs for any errors
2. Go to https://github.com/orgs/storacha/projects/1
3. Look for issues that were moved to "Stuck / Dead Issue" status
4. Check that comments were added explaining why issues were moved
5. Review any duplicate groups that were identified

## Adjusting Configuration

Edit `.github/workflows/daily-maintenance.yml` to adjust:

```yaml
env:
  STALENESS_THRESHOLD_DAYS: 180  # Change to 90, 365, etc.
  DUPLICATE_SIMILARITY: 0.85     # Higher = stricter (0.0-1.0)
```

To change the schedule:

```yaml
schedule:
  # Run every day at 9 AM UTC
  - cron: '0 9 * * *'

  # Or change to run weekly on Mondays:
  # - cron: '0 9 * * 1'
```

## Troubleshooting

### "Failed to fetch project metadata"
- Verify the GitHub token has `project` permissions
- Check that PROJECT_NUMBER is correct (should be `1`)
- Ensure token has organization access

### "GEMINI_API_KEY environment variable is required"
- Verify the secret is named exactly `GEMINI_API_KEY`
- Check that the secret value is set correctly

### Issues not being found
- Confirm issues are actually in the Project (not just the repo)
- Verify Status field is set to "Backlog"
- Check that the Status field exists and is named "Status" (case-sensitive)

### Rate Limits
- GitHub Actions: 1,000 API requests per hour per token
- Gemini Free Tier: 15 requests per minute, 1,500 per day
- If you hit limits, reduce frequency or increase sleep delays

## Monitoring

The agent will:
- Run daily at 9 AM UTC
- Create workflow run logs with full details
- Exit with error code if critical issues occur
- Continue on non-critical errors (duplicate detection failures)

Check the Actions tab regularly to ensure it's running successfully.

## Next Steps

- Monitor the first few runs carefully
- Adjust thresholds based on your backlog's behavior
- Consider creating a separate "Icebox" status for permanent storage
- Add custom labels for different types of stale issues
- Extend the agent with additional rules (priority-based, label-based, etc.)

## Support

For issues or questions:
- Check the logs in the Actions tab
- Review the main README.md for detailed documentation
- Open an issue in the repository
