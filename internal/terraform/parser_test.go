package terraform

import (
	"testing"
)

func TestExtractHCL_FencedWithLanguageHint(t *testing.T) {
	input := "```hcl\nterraform {\n  required_providers {}\n}\n```"
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected non-empty HCL, got empty string")
	}
	want := "terraform {\n  required_providers {}\n}"
	if got != want {
		t.Errorf("ExtractHCL fenced+hint:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestExtractHCL_FencedWithTerraformHint(t *testing.T) {
	input := "```terraform\nterraform {\n}\n```"
	got := ExtractHCL(input)
	want := "terraform {\n}"
	if got != want {
		t.Errorf("ExtractHCL fenced+terraform hint:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestExtractHCL_FencedNoLanguageHint(t *testing.T) {
	input := "```\nterraform {\n  required_providers {}\n}\n```"
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected non-empty HCL, got empty string")
	}
	want := "terraform {\n  required_providers {}\n}"
	if got != want {
		t.Errorf("ExtractHCL fenced no hint:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestExtractHCL_RawHCLNoFences(t *testing.T) {
	input := "terraform {\n  required_providers {\n    aws = {\n      source = \"hashicorp/aws\"\n    }\n  }\n}"
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected non-empty HCL for raw input, got empty string")
	}
	if got != input {
		t.Errorf("ExtractHCL raw HCL:\ngot:  %q\nwant: %q", got, input)
	}
}

func TestExtractHCL_ResourceBlockNoFences(t *testing.T) {
	input := `resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-bucket"
}`
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected non-empty HCL for resource block, got empty string")
	}
}

func TestExtractHCL_ProviderBlockNoFences(t *testing.T) {
	input := `provider "aws" {
  region = "us-east-1"
}`
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected non-empty HCL for provider block, got empty string")
	}
}

func TestExtractHCL_ProseOnly_ReturnsEmpty(t *testing.T) {
	input := "Sure, here is your code: bla bla bla"
	got := ExtractHCL(input)
	if got != "" {
		t.Errorf("ExtractHCL prose-only: expected empty string, got %q", got)
	}
}

func TestExtractHCL_ExplanationThenRawHCL(t *testing.T) {
	input := "Here is your Terraform HCL for an S3 bucket:\n\nterraform {\n  required_providers {}\n}"
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected HCL extracted from explanation+HCL, got empty string")
	}
	// Must contain the HCL portion
	if got != "terraform {\n  required_providers {}}" && got != "terraform {\n  required_providers {}\n}" {
		// Accept either — just ensure it starts with terraform {
		if len(got) < 11 || got[:11] != "terraform {" {
			t.Errorf("ExtractHCL explanation+HCL: expected to start with 'terraform {', got %q", got)
		}
	}
}

func TestExtractHCL_ExplanationThenFencedHCL(t *testing.T) {
	input := "Sure! Here is the code:\n```hcl\nterraform {\n  required_providers {}\n}\n```"
	got := ExtractHCL(input)
	if got == "" {
		t.Fatal("expected HCL extracted from explanation+fenced, got empty string")
	}
	want := "terraform {\n  required_providers {}\n}"
	if got != want {
		t.Errorf("ExtractHCL explanation+fenced:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestExtractHCL_EmptyInput(t *testing.T) {
	got := ExtractHCL("")
	if got != "" {
		t.Errorf("ExtractHCL empty input: expected empty string, got %q", got)
	}
}

func TestExtractHCL_WhitespaceOnly(t *testing.T) {
	got := ExtractHCL("   \n\t\n   ")
	if got != "" {
		t.Errorf("ExtractHCL whitespace-only: expected empty string, got %q", got)
	}
}

func TestExtractHCL_PreservesInternalNewlines(t *testing.T) {
	hcl := `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.6"
}

resource "aws_s3_bucket" "example" {
  bucket = "my-example-bucket"

  tags = {
    Project   = "infrasage"
    ManagedBy = "terraform"
  }
}`
	input := "```hcl\n" + hcl + "\n```"
	got := ExtractHCL(input)
	if got != hcl {
		t.Errorf("ExtractHCL multi-block:\ngot:  %q\nwant: %q", got, hcl)
	}
}
