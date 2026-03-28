terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  
  # For production, utilize a remote backend:
  # backend "s3" {
  #   bucket = "go-fintech-tf-state"
  #   key    = "state/terraform.tfstate"
  #   region = "us-east-1"
  # }
}

provider "aws" {
  region = var.aws_region
}

# ----------------------------------------------------
# Architectural Components
# ----------------------------------------------------

module "rds_postgres" {
  source      = "./modules/rds"
  environment = var.environment
}

module "msk_kafka" {
  source      = "./modules/msk"
  environment = var.environment
  vpc_id      = var.vpc_id
}

module "ecs_services" {
  source      = "./modules/ecs"
  environment = var.environment
}
