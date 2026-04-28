# CICD.md — CI/CD Pipeline Instructions

> Read this before writing anything in `.github/workflows/` or `cmd/deploy.go`.

---

## What CI/CD Does In This Project

CI/CD in InfraSage handles two things:

**1. Validation on every push (infrasage-ci.yml):**
When a `.tf` file is pushed or a PR is opened, GitHub Actions automatically:
- Runs the three security scanners (same tools as local CLI)
- Runs Terraform plan via Digger
- Comments the plan result on the PR

**2. Drift detection every 6 hours (drift-check.yml):**
A scheduled workflow runs `terraform plan` against real AWS state.
- Exit code 0: no changes, all good
- Exit code 2: infrastructure has drifted from the `.tf` files
- On drift: auto-creates a PR to signal that remediation is needed

The Go CLI's `deploy` command triggers this whole pipeline by pushing a branch and creating a PR.

---

## What Digger Is And Why We Use It

Digger is a GitOps tool for Terraform. Without it:
- You'd need to run `terraform apply` manually after every PR merge
- AWS credentials would need to be in every developer's terminal
- No audit trail of who applied what

With Digger:
- Terraform plan runs automatically in GitHub Actions
- Digger posts the plan output as a PR comment
- On PR merge, Digger runs `terraform apply` automatically **when AWS secrets are configured**
- All apply history is in the GitHub PR timeline

**How to set up Digger (human does this once):**
1. Go to https://digger.dev
2. Sign up with GitHub
3. Install the Digger GitHub App on your repository
4. Add `DIGGER_TOKEN` to GitHub repository secrets

Without the Digger token, the Digger action in CI will fail. This is acceptable for Phase 1 demo — the scan jobs will still work.

---

## GitHub Actions Workflow: infrasage-ci.yml

### Trigger Configuration

```yaml
on:
  push:
    branches: [main]
    paths:
      - "**.tf"        # Only trigger when .tf files change
  pull_request:
    branches: [main]
    paths:
      - "**.tf"
```

**Why `paths: ["**.tf"]`:** We don't want CI to run when README or Go code changes. CI should only run when infrastructure code changes.

### Job 1: scan

```yaml
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Checkov Scan
        uses: bridgecrewio/checkov-action@master
        with:
          directory: .
          quiet: true
          output_format: cli
        continue-on-error: false   # Fail the job if Checkov finds issues
      
      - name: tfsec Scan
        uses: aquasecurity/tfsec-action@v1.0.0
        with:
          working_directory: .
          format: lovely
        continue-on-error: false
      
      - name: Terrascan Scan
        uses: accurics/terrascan-action@main
        with:
          iac_type: terraform
          iac_version: v14
          policy_type: aws
        continue-on-error: false
```

**Note:** `continue-on-error: false` means the job fails if any scanner finds issues. This is intentional — it enforces security as a gate.

### Job 2: plan (runs only after scan passes)

```yaml
  plan:
    runs-on: ubuntu-latest
    needs: [scan]             # Only runs if scan job passes
    permissions:
      contents: write         # Digger needs to write PR comments
      pull-requests: write
      id-token: write
    steps:
      - uses: actions/checkout@v4
      
      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: "1.6.0"
      
      - name: Digger Plan
        uses: diggerhq/digger@v0.3.0
        with:
          setup-aws: true
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          DIGGER_TOKEN: ${{ secrets.DIGGER_TOKEN }}
```

---

## GitHub Actions Workflow: drift-check.yml

```yaml
name: Drift Detection

on:
  schedule:
    - cron: "0 */6 * * *"   # Every 6 hours: midnight, 6am, noon, 6pm UTC
  workflow_dispatch:          # Allow manual trigger from GitHub UI

jobs:
  drift:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write

    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-terraform@v3

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - name: Check for drift
        id: drift
        run: |
          terraform init -backend-config="..." 
          terraform plan -detailed-exitcode -out=tfplan
        continue-on-error: true

      - name: Create drift PR if needed
        if: steps.drift.outputs.exitcode == '2'
        uses: peter-evans/create-pull-request@v5
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          commit-message: "chore: infrastructure drift detected"
          title: "⚠️ Infrastructure Drift Detected"
          body: |
            Terraform plan detected differences between state and actual infrastructure.
            Review and apply the remediation plan.
            
            Run: `infrasage drift` locally to see the full plan output.
          branch: infrasage/drift-remediation
          base: main
```

**Important about `steps.drift.outputs.exitcode`:**
The `continue-on-error: true` on the plan step means the job doesn't fail on exit code 2. But we need to capture the exit code to check it in the next step. Use this pattern in the run step:
```yaml
run: |
  terraform plan -detailed-exitcode
  echo "exitcode=$?" >> $GITHUB_OUTPUT
```

---

## The `deploy` Go Command — What It Does

`infrasage deploy infra.tf` does these things in order:

1. **Runs scan first** — aborts if CRITICAL findings. You don't push insecure code.

2. **Creates a branch** named `infrasage/deploy-<sanitized-filename>-<unix-timestamp>`
   - Example: `infrasage/deploy-infra-tf-1718283600`
   - The timestamp prevents duplicate branch names on multiple deploys

3. **Stages and commits** the `.tf` file with message:
   `"feat(infrasage): add generated infrastructure <filename>"`

4. **Pushes to GitHub** (`git push -u origin <branch>`)

5. **Creates a Pull Request** via GitHub REST API:
   - Title: `"Deploy: <filename>"`
   - Body includes: the scan summary (how many checks passed/failed)
   - Base branch: `main`

6. **Prints the PR URL** and tells user what to expect from Digger

**GitHub API call for creating a PR:**
```
POST https://api.github.com/repos/{owner}/{repo}/pulls
Authorization: token {GITHUB_TOKEN}
Content-Type: application/json

{
  "title": "Deploy: infra.tf",
  "body": "Generated by InfraSage\n\nScan summary:\n- Checkov: 12 passed, 2 failed\n...",
  "head": "infrasage/deploy-infra-tf-1718283600",
  "base": "main"
}
```

The `GITHUB_REPO` env var must be in `owner/repo` format, e.g., `johndoe/infrasage-demo`.

---

## GitHub Repository Secrets To Set Up

The human must add these in GitHub → Repository → Settings → Secrets and variables → Actions:

| Secret Name | Value | Used By |
|---|---|---|
| `AWS_ACCESS_KEY_ID` | AWS IAM user key | Digger plan/apply, drift check |
| `AWS_SECRET_ACCESS_KEY` | AWS IAM user secret | Digger plan/apply, drift check |
| `DIGGER_TOKEN` | From digger.dev account | Digger action |

The `GITHUB_TOKEN` secret is auto-provided by GitHub Actions — no setup needed.

InfraSage can sync AWS secrets for you:
```
infrasage config --aws
```
This uses the GitHub CLI (`gh`) to upload your local AWS credentials to repository secrets.

---

## Digger Config File

**What to create:** `digger.yml` in the repository root.

```yaml
projects:
  - name: infrasage-demo
    dir: .
    workspace: default
    workflow: default
    apply_after_merge: true
    generate_projects:
      files_changed:
        patterns:
          - "**/*.tf"
```

**`apply_after_merge: true`** means Digger will run `terraform apply` automatically when the PR is merged (and AWS secrets are present). This is the GitOps automation.

Without this, Digger only runs `plan`. The human would need to manually approve the apply in the Digger dashboard.

---

## AWS Setup For Free Tier (For Demo Purposes)

The project targets AWS Free Tier resources to avoid charges during development:

| Resource | Free Tier Limit | Used For |
|---|---|---|
| S3 | 5GB storage, 20K GET, 2K PUT | Demo IaC outputs |
| EC2 t2.micro | 750 hours/month | Demo compute |
| Terraform state | S3 bucket | Storing terraform.tfstate |

**IAM user to create (human does this in AWS Console):**
1. Create IAM user `infrasage-demo`
2. Attach policy: `AmazonS3FullAccess`, `AmazonEC2FullAccess` (for demo)
3. In production: use least-privilege custom policy
4. Generate access key → save to GitHub secrets

**Terraform state backend:**
For demo, use local state (no `backend {}` block in generated Terraform). For cloud demo, add an S3 backend.
