# InfraSage

InfraSage is a Go CLI that converts natural language prompts into Terraform, scans output with Checkov/tfsec/Terrascan, supports LLM-assisted remediation, and ships CI-ready workflows with Prometheus/Grafana monitoring.

## Architecture

```
User prompt
   |
   v
infrasage ask ---> Ollama (local, native macOS)
   |
   +--> Terraform HCL file
   |
   +--> Scanner pipeline (checkov + tfsec + terrascan)
   |       |
   |       +--> unified report + checkov JSON output
   |
   +--> fix/apply/deploy flow

Monitoring:
CLI metrics (:2112) --> Prometheus (docker) --> Grafana (docker)

CI/CD:
PR/Push .tf --> GitHub Actions --> scanners + terraform dry-run comments
```

## Prerequisites

- macOS with Go 1.22+
- Docker Desktop
- Terraform CLI
- Ollama
- Model: `qwen2.5-coder:3b`

Install basics:

```bash
brew install go terraform ollama
ollama pull qwen2.5-coder:3b
```

## Quick Start

```bash
git clone <your-repo-url>
cd infrasage
cp .env.example .env
make setup
./bin/infrasage ask "create a secure S3 bucket"
```

## Commands

| Command | What it does |
|---|---|
| `infrasage stack up` | Starts scanners + Prometheus + Grafana and checks Ollama |
| `infrasage stack down` | Stops stack containers |
| `infrasage stack status` | Shows container status |
| `infrasage ask "..."` | Generates Terraform and auto-scans by default |
| `infrasage ask "..." --out file.tf` | Writes output to custom file |
| `infrasage ask "..." --no-scan` | Skips auto-scan |
| `infrasage scan file.tf` | Runs all three scanners in parallel |
| `infrasage fix file.tf` | Uses local LLM to rewrite Terraform from Checkov findings |
| `infrasage apply file.tf` | Runs isolated terraform init/validate/plan/apply workflow |
| `infrasage deploy file.tf` | Pre-scan gate, branch push, and GitHub PR creation |
| `infrasage drift` | Runs terraform drift check |
| `infrasage monitor open` | Opens Grafana |
| `infrasage monitor metrics` | Prints Prometheus metrics from CLI |
| `infrasage model pull` | Pulls configured Ollama model |
| `infrasage model status` | Lists local Ollama models |

## No-AWS Mode

This repository supports simulation-first CI/CD without an AWS account:

- Security scans run in GitHub Actions for `.tf` changes.
- Terraform dry-run comments are posted on PRs.
- Real cloud apply/drift can be enabled later by adding AWS secrets.

## Monitoring Endpoints

- Prometheus: `http://localhost:9091`
- Grafana: `http://localhost:3000`
- CLI metrics: `http://localhost:2112/metrics`

## Demo Capture

Record a short terminal run that includes:

1. `infrasage stack up`
2. `infrasage ask "create an insecure s3 bucket" --out demo.tf`
3. `infrasage fix demo.tf --out demo.fixed.tf`
4. `infrasage apply demo.tf`
5. `infrasage deploy demo.tf`
