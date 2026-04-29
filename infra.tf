terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.6"
}

provider "aws" {
  region = "us-west-2"
}

resource "aws_s3_bucket" "example" {
  bucket = "example-bucket-name"
  
  tags = {
    Project        = "infrasage"
    ManagedBy      = "terraform"
  }
  
  server_side_encryption_configuration {
    rule {
      application = "aws:kms"
      key_id      = "alias/aws/s3"
    }
  }
  
  versioning {
    enabled = true
  }
  
  block_public_acls        = true
  block_public_policy      = true
  ignore_public_acls       = true
  restrict_public_buckets  = true
}
