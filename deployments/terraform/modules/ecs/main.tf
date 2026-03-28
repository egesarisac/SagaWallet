variable "environment" {
  type = string
}

# Create pure ECS cluster
resource "aws_ecs_cluster" "fintech_cluster" {
  name = "go-fintech-${var.environment}"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = {
    Environment = var.environment
  }
}

# Note: Task definitions / services for Wallet, Transaction, and Notification
# should be implemented mapping specific ECR container images and load balancers.
