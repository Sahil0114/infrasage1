provider "aws" {
  region = "us-west-2"
  required_version = ">= 1.6"
}

resource "aws_s3_bucket" "example_bucket" {
  bucket = "my-infra-bucket"

  acl                  = "private"
  block_public_acls    = true
  block_public_policy   = true
  ignore_public_acls     = true
  restrict_public_buckets = true

  server_side_encryption_configuration {
    rule {
      apply_server_side_encryption_by_default {
        algorithm = "AES256"
      }
    }
  }

  versioning {
    enabled = true
  }

  tags = {
    Project = "infrasage"
    ManagedBy = "terraform"
  }
}
