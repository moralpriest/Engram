#!/bin/bash
# Configure branch protection rules for Engram repository
# This script should be run by a repository admin

set -e

REPO="moralpriest/Engram"

echo "Configuring branch protection rules for Engram..."
echo "Repository: $REPO"
echo ""

# Check if gh CLI is installed and authenticated
if ! command -v gh &> /dev/null; then
    echo "Error: GitHub CLI (gh) is not installed"
    echo "Install from: https://cli.github.com/"
    exit 1
fi

if ! gh auth status &> /dev/null; then
    echo "Error: Not authenticated with GitHub CLI"
    echo "Run: gh auth login"
    exit 1
fi

# Configure main branch protection
echo "Setting up main branch protection..."
gh api repos/$REPO/branches/main/protection \
  --method PUT \
  --input - <<EOF
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Lint",
      "Build (ubuntu-latest)",
      "Build (macos-latest)",
      "Build (windows-latest)",
      "Test",
      "Secret Scanning",
      "Go Vulnerability Check",
      "Go Security Analysis",
      "CodeQL Analysis",
      "Semgrep Analysis",
      "Trivy Filesystem Scan",
      "Lint Markdown",
      "Check Links",
      "Spell Check"
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": true
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_linear_history": true,
  "required_signatures": true
}
EOF

echo "✅ Main branch protection configured"

# Configure dev branch protection
echo "Setting up dev branch protection..."
gh api repos/$REPO/branches/dev/protection \
  --method PUT \
  --input - <<EOF
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Lint",
      "Build (ubuntu-latest)",
      "Build (macos-latest)",
      "Build (windows-latest)",
      "Test",
      "Secret Scanning",
      "Go Vulnerability Check",
      "Go Security Analysis"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF

echo "✅ Dev branch protection configured"

# Set up ruleset for workflow files
echo "Setting up ruleset for workflow protection..."
gh api repos/$REPO/rulesets \
  --method POST \
  --input - <<EOF
{
  "name": "Protect CI/CD Workflows",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {
      "include": ["~DEFAULT_BRANCH"],
      "exclude": []
    }
  },
  "rules": [
    {
      "type": "required_status_checks",
      "parameters": {
        "required_status_checks": [
          {"context": "Lint", "integration_id": 15368},
          {"context": "Test", "integration_id": 15368}
        ],
        "strict_required_status_checks_policy": true
      }
    },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": true,
        "require_code_owner_review": true
      }
    }
  ],
  "bypass_actors": []
}
EOF

echo "✅ Ruleset configured"

echo ""
echo "========================================"
echo "Branch protection setup complete!"
echo ""
echo "Summary:"
echo "  - main: Strict protection, requires all checks + code owner review"
echo "  - dev: Moderate protection, requires checks + 1 review"
echo "  - Workflow changes require CODEOWNERS approval"
echo ""
echo "Note: Repository admin bypass is disabled for main branch"
echo "========================================"
