variable "aws_region" {
  description = "AWS region for deployments"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (e.g. staging, prod)"
  type        = string
  default     = "staging"
}

variable "vpc_id" {
  description = "VPC ID for resources"
  type        = string
  default     = "vpc-mock123"
}
