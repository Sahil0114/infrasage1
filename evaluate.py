#!/usr/bin/env python3
"""Evaluate Terraform generation quality via local Ollama API.

Usage:
  pip install requests python-hcl2
  python evaluate.py
"""

from __future__ import annotations

import json
import re
import statistics
from dataclasses import dataclass
from typing import List, Tuple

import requests

try:
    import hcl2
except ImportError:  # pragma: no cover
    hcl2 = None

OLLAMA_URL = "http://localhost:11434/api/generate"
MODEL = "qwen2.5-coder:3b"
TIMEOUT_SECONDS = 120

SYSTEM_PROMPT = """You are an expert Terraform engineer. Your only job is to output valid Terraform HCL code.

Rules you must follow without exception:
1. Output ONLY raw HCL code. No markdown. No explanations. No commentary.
2. Start your response with the first line of HCL code.
3. Always include a terraform { required_providers { ... } } block.
4. Always include required_version = \">= 1.6\" inside the terraform block.
5. Use the AWS provider unless the user explicitly says otherwise.
6. Follow these security defaults:
   - S3 buckets: block_public_acls = true, block_public_policy = true,
                 ignore_public_acls = true, restrict_public_buckets = true
   - S3 buckets: enable server_side_encryption_configuration
   - S3 buckets: enable versioning
   - Security groups: no ingress 0.0.0.0/0 on port 22 unless asked
   - IAM: no wildcard (*) actions or resources unless asked
7. Add these tags to every resource:
   tags = { Project = \"infrasage\", ManagedBy = \"terraform\" }
8. Use snake_case for all resource and variable names.
9. The AWS provider version must be ~> 5.0.
10. Do not use deprecated attributes."""

PROMPTS: List[str] = [
    "create a secure S3 bucket with versioning and encryption",
    "create an EC2 instance with least-privilege security group",
    "create a VPC with public and private subnets",
    "create an RDS PostgreSQL instance with backups enabled",
    "create an IAM role for lambda without wildcard permissions",
    "create an S3 bucket for logs and lifecycle rules",
    "create an application load balancer and target group",
    "create a DynamoDB table with encryption and tags",
    "create a CloudWatch log group with retention policy",
    "create an SQS queue with dead-letter queue",
    "create an SNS topic and subscribe an SQS queue",
    "create an autoscaling group for web servers",
    "create a NAT gateway architecture for private subnets",
    "create an ECR repository with scan on push",
    "create a lambda function with IAM role and logs",
    "create an S3 static website but keep it private",
    "create a secure bastion host with restricted SSH",
    "create a KMS key and use it for S3 encryption",
    "create an ECS cluster with Fargate service",
    "create a minimal Terraform file for a tagged resource",
]


@dataclass
class EvalResult:
    prompt: str
    score: int
    notes: List[str]


def extract_hcl(raw: str) -> str:
    text = (raw or "").strip()
    fence = re.search(r"(?s)```(?:hcl|terraform|tf)?\n?(.*?)```", text)
    if fence:
        return fence.group(1).strip()
    if any(token in text for token in ["terraform {", 'resource "', 'provider "']):
        return text
    return ""


def generate(prompt: str) -> str:
    payload = {
        "model": MODEL,
        "prompt": prompt,
        "system": SYSTEM_PROMPT,
        "stream": False,
        "options": {"temperature": 0.1, "num_ctx": 4096},
    }
    resp = requests.post(OLLAMA_URL, json=payload, timeout=TIMEOUT_SECONDS)
    resp.raise_for_status()
    data = resp.json()
    return data.get("response", "")


def score_hcl(hcl: str) -> Tuple[int, List[str]]:
    score = 0
    notes: List[str] = []

    if "terraform {" in hcl:
        score += 1
    else:
        notes.append("missing terraform block")

    if "required_providers" in hcl:
        score += 1
    else:
        notes.append("missing required_providers")

    if "required_version" in hcl:
        score += 1
    else:
        notes.append("missing required_version")

    if re.search(r"(?m)^\s*tags\s*=\s*\{", hcl):
        score += 1
    else:
        notes.append("missing tags")

    if hcl2 is None:
        notes.append("python-hcl2 not installed, parse score skipped")
    else:
        try:
            hcl2.loads(hcl)
            score += 2
        except Exception as exc:  # pragma: no cover
            notes.append(f"invalid hcl parse: {exc}")

    return score, notes


def main() -> None:
    results: List[EvalResult] = []

    for i, prompt in enumerate(PROMPTS, start=1):
        try:
            raw = generate(prompt)
            hcl = extract_hcl(raw)
            if not hcl:
                results.append(EvalResult(prompt, 0, ["model did not return HCL"]))
                continue
            score, notes = score_hcl(hcl)
            results.append(EvalResult(prompt, score, notes))
        except Exception as exc:  # pragma: no cover
            results.append(EvalResult(prompt, 0, [f"generation error: {exc}"]))

        print(f"[{i:02d}/{len(PROMPTS)}] done")

    print("\nPrompt | Score | Notes")
    print("-" * 90)
    for r in results:
        note_text = "; ".join(r.notes) if r.notes else "ok"
        short_prompt = (r.prompt[:40] + "...") if len(r.prompt) > 43 else r.prompt
        print(f"{short_prompt:<45} | {r.score}/6 | {note_text}")

    scores = [r.score for r in results]
    avg = statistics.mean(scores) if scores else 0.0
    print("\nAverage score: {:.2f}/6".format(avg))

    low = [r for r in results if r.score < 3]
    if low:
        print("\nPrompts below 3/6:")
        for r in low:
            print(f"- {r.prompt}")


if __name__ == "__main__":
    main()
