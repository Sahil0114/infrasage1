terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

resource "aws_s3_bucket" "example_bucket" {
  bucket        = "example-bucket-1234567890"
  acl          = "private"
  block_public_acls = true
  block_public_policy = true
  ignore_public_acls = true
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

  lifecycle_rule {
    id     = "DeleteOldObjects"
    prefix = "old/"
    expiration {
      days = 30
    }
  }

  tags = {
    Project = "infrasage"
    ManagedBy = "terraform"
  }
}

resource "aws_s3_bucket_policy" "example_bucket_policy" {
  bucket = aws_s3_bucket.example_bucket.bucket

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Deny"
        Principal = "*"
        Action    = ["s3:GetObject"]
        Resource  = "${aws_s3_bucket.example_bucket.arn}/*"
        Condition = {
          StringNotEquals = {
            s3:ExistingObjectTag/AccessControlPolicy = "true"
          }
        }
      },
      {
        Effect   = "Deny"
        Principal = "*"
        Action    = ["s3:GetBucketAcl"]
        Resource  = aws_s3_bucket.example_bucket.arn
      },
      {
        Effect   = "Deny"
        Principal = "*"
        Action    = ["s3:PutObjectAcl"]
        Resource  = "${aws_s3_bucket.example_bucket.arn}/*"
      }
    ]
  })
}

resource "aws_s3_bucket_notification" "example_bucket_notification" {
  bucket = aws_s3_bucket.example_bucket.bucket

  lambda_function_configuration {
    event_type           = "s3:ObjectCreated:*"
    filter_prefix        = ""
    filter_suffix        = ""
    lambda_function_arn = aws_lambda_function.example_lambda.arn
  }
}

resource "aws_lambda_function" "example_lambda" {
  filename      = "path/to/your/lambdafile.zip"
  function_name = "example-lambda-function"
  role          = aws_iam_role.example_lambda_role.arn
  handler       = "index.handler"

  runtime = "python3.8"

  environment {
    variables = {
      BUCKET_NAME = aws_s3_bucket.example_bucket.bucket
    }
  }

  depends_on = [aws_iam_role_policy_attachment.example_lambda_policy_attachment]
}

resource "aws_iam_role" "example_lambda_role" {
  name = "example-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
        Action = ["sts:AssumeRole"]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "example_lambda_policy_attachment" {
  role       = aws_iam_role.example_lambda_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}