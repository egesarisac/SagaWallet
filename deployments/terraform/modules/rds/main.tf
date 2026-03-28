variable "environment" {
  type = string
}

resource "aws_db_instance" "fintech_db" {
  allocated_storage      = 20
  engine                 = "postgres"
  engine_version         = "16"
  instance_class         = "db.t3.micro"
  db_name                = "fintech_${var.environment}"
  username               = "fintech_admin"
  password               = "use_secrets_manager_in_production"
  skip_final_snapshot    = true
  publicly_accessible    = false

  tags = {
    Environment = var.environment
    Service     = "Database"
  }
}
