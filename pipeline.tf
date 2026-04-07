provider "aws" {
  region = "us-west-2"
  required_version = ">= 1.6"
}

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

resource "aws_s3_bucket" "insecure_public_bucket" {
  bucket = "my_insecure_public_bucket"

  tags = {
    Project = "infrasage"
    ManagedBy = "terraform"
  }

  block_public_acls   = true
  block_public_policy = true
  ignore_public_acls  = true
  restrict_public_buckets = true

  server_side_encryption_configuration {
    rule {
      apply_server_side_encryption_by_default {
        sse_algorithm = "AES256"
      }
    }
  }

  versioning {
    enabled = true
  }
}
