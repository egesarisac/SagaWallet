terraform {
  required_providers {
    neon = {
      source  = "kislerdm/neon"
      version = "~> 0.2"
    }
  }
}

variable "neon_api_key" {
  type      = string
  sensitive = true
}

variable "existing_project_id" {
  type    = string
  default = ""
}

# Use existing project ID
resource "neon_branch" "main" {
  project_id = var.existing_project_id
  name       = "main"
}

resource "neon_endpoint" "main" {
  project_id = var.existing_project_id
  branch_id  = neon_branch.main.id
  type       = "read_write"
}

# wallet_db
resource "neon_database" "wallet" {
  project_id = var.existing_project_id
  branch_id  = neon_branch.main.id
  name       = "wallet_db"
  owner_name = "neondb_owner"
}

resource "neon_role" "wallet" {
  project_id = var.existing_project_id
  branch_id  = neon_branch.main.id
  name       = "wallet_user"
}

# transaction_db
resource "neon_database" "transaction" {
  project_id = var.existing_project_id
  branch_id  = neon_branch.main.id
  name       = "transaction_db"
  owner_name = "neondb_owner"
}

resource "neon_role" "transaction" {
  project_id = var.existing_project_id
  branch_id  = neon_branch.main.id
  name       = "transaction_user"
}

output "wallet_db_url" {
  value     = "postgres://${neon_role.wallet.name}:${neon_role.wallet.password}@${neon_endpoint.main.host}/${neon_database.wallet.name}?sslmode=require"
  sensitive = true
}

output "transaction_db_url" {
  value     = "postgres://${neon_role.transaction.name}:${neon_role.transaction.password}@${neon_endpoint.main.host}/${neon_database.transaction.name}?sslmode=require"
  sensitive = true
}
