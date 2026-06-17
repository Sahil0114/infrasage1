provider "aws" {
  region = "us-west-2"
  required_version = ">= 1.6"
}

terraform {
  required_providers {
    aws = "~> 5.0"
  }
}

resource "aws_s3_bucket" "insecure_public_bucket" {
  bucket = "my-insecure-public-bucket"

  tags = {
    Project = "infrasage"
    ManagedBy = "terraform"
  }

  acl = "public-read-write"

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
