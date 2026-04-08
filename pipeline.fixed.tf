terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

resource "aws_s3_bucket" "secure_bucket" {
  bucket = "my_secure_bucket"

  tags = {
    Project = "infrasage"
    ManagedBy = "terraform"
  }

  acl = "private"
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

  lifecycle_rule {
    id     = "DeleteOldObjects"
    prefix = "old/"
    expiration {
      days = 30
    }
  }

  logging {
    target_bucket = aws_s3_bucket.secure_bucket.id
    target_prefix = "logs/"
  }

  replication_configuration {
    role = aws_iam_role.s3_replication_role.arn

    rule {
      id          = "ReplicateToAnotherRegion"
      destination = {
        bucket = "my_secure_bucket_copy"
        storage_class = "STANDARD"
      }
    }
  }
}

resource "aws_s3_bucket_policy" "secure_bucket_policy" {
  bucket = aws_s3_bucket.secure_bucket.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Deny"
        Principal = "*"
        Action    = ["s3:GetObject", "s3:PutObject"]
        Resource = "${aws_s3_bucket.secure_bucket.arn}/*"
        Condition = {
          StringNotEquals = {
            s3:x-amz-acl = ["public-read", "public-read-write"]
          }
        }
      },
    ]
  })
}

resource "aws_iam_role" "s3_replication_role" {
  name = "s3-replication-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Principal = {
          Service = ["s3.amazonaws.com"]
        }
        Action    = ["sts:AssumeRole"]
      },
    ]
  })

  policies = [
    jsonencode({
      Version = "2012-10-17"
      Statement = [
        {
          Effect   = "Allow"
          Action    = ["s3:GetObject", "s3:PutObject"]
          Resource = "${aws_s3_bucket.secure_bucket.arn}/*"
        },
      ]
    }),
  ]
}

resource "aws_iam_role_policy_attachment" "s3_replication_role_policy_attachment" {
  role       = aws_iam_role.s3_replication_role.name
  policy_arn = aws_iam_policy.s3_replication_policy.arn
}

resource "aws_s3_bucket_notification_configuration" "secure_bucket_notification" {
  bucket = aws_s3_bucket.secure_bucket.id

  lambda_function_configuration {
    events = ["s3:ObjectCreated:*"]
    filter_prefix = ""
    filter_suffix = ""

    destination = {
      lambda_function_arn = aws_lambda_function.s3_notification_lambda.arn
    }
  }

  s3_event_destination {
    bucket = aws_s3_bucket.secure_bucket.id

    event_filter {
      prefix = "logs/"
    }

    destination = {
      s3_bucket_arn = aws_s3_bucket.secure_bucket_copy.arn
    }
  }
}

resource "aws_lambda_function" "s3_notification_lambda" {
  filename      = "s3-notification-lambda.zip"
  function_name = "s3-notification-lambda"
  role         = aws_iam_role.s3_notification_lambda_role.arn
  handler       = "index.handler"

  runtime = "python3.8"

  environment = {
    variables = {
      BUCKET_NAME = aws_s3_bucket.secure_bucket.id
    }
  }

  timeout     = 10
  memory_size = 512

  depends_on = [aws_iam_role_policy_attachment.s3_notification_lambda_policy_attachment]
}

resource "aws_iam_role" "s3_notification_lambda_role" {
  name = "s3-notification-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Principal = {
          Service = ["lambda.amazonaws.com"]
        }
        Action    = ["sts:AssumeRole"]
      },
    ]
  })

  policies = [
    jsonencode({
      Version = "2012-10-17"
      Statement = [
        {
          Effect   = "Allow"
          Action    = ["s3:GetObject", "s3:PutObject"]
          Resource = "${aws_s3_bucket.secure_bucket.arn}/*"
        },
        {
          Effect   = "Allow"
          Action    = ["logs:*"]
          Resource = "*"
        },
      ]
    }),
  ]
}

resource "aws_iam_role_policy_attachment" "s3_notification_lambda_policy_attachment" {
  role       = aws_iam_role.s3_notification_lambda_role.name
  policy_arn = aws_iam_policy.s3_notification_lambda_policy.arn
}