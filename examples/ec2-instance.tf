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
  region = "us-east-1"
}

resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Project   = "infrasage"
    ManagedBy = "terraform"
    Name      = "infrasage-vpc"
  }
}

resource "aws_subnet" "private" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "us-east-1a"
  map_public_ip_on_launch = false

  tags = {
    Project   = "infrasage"
    ManagedBy = "terraform"
    Name      = "infrasage-private-subnet"
  }
}

resource "aws_security_group" "ec2_sg" {
  name        = "infrasage-ec2-sg"
  description = "Security group for InfraSage EC2 instance — no inbound SSH from 0.0.0.0/0"
  vpc_id      = aws_vpc.main.id

  # Allow outbound HTTPS for package updates and AWS API calls
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Outbound HTTPS"
  }

  # Allow outbound HTTP (for package manager)
  egress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Outbound HTTP"
  }

  tags = {
    Project   = "infrasage"
    ManagedBy = "terraform"
    Name      = "infrasage-ec2-sg"
  }
}

resource "aws_instance" "app" {
  ami                    = "ami-0c02fb55956c7d316" # Amazon Linux 2023 us-east-1
  instance_type          = "t3.micro"
  subnet_id              = aws_subnet.private.id
  vpc_security_group_ids = [aws_security_group.ec2_sg.id]

  # Require IMDSv2 (mitigates SSRF attacks against instance metadata service)
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  # Enable detailed monitoring
  monitoring = true

  # Encrypt the root EBS volume
  root_block_device {
    volume_type           = "gp3"
    volume_size           = 20
    encrypted             = true
    delete_on_termination = true

    tags = {
      Project   = "infrasage"
      ManagedBy = "terraform"
      Name      = "infrasage-root-volume"
    }
  }

  tags = {
    Project   = "infrasage"
    ManagedBy = "terraform"
    Name      = "infrasage-app-server"
  }
}
