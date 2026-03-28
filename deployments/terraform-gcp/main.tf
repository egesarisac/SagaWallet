terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    confluent = {
      source  = "confluentinc/confluent"
      version = "~> 2.0"
    }
    neon = {
      source  = "kislerdm/neon"
      version = "~> 0.2"
    }
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.gcp_region
}

provider "confluent" {
  cloud_api_key    = var.confluent_cloud_api_key
  cloud_api_secret = var.confluent_cloud_api_secret
}

provider "neon" {
  api_key = var.neon_api_key
}

# ----------------------------------------
# Artifact Registry (Docker image storage)
# ----------------------------------------
resource "google_artifact_registry_repository" "fintech" {
  repository_id = "go-fintech"
  format        = "DOCKER"
  location      = var.gcp_region
  description   = "Docker images for go-fintech microservices"
}

# ----------------------------------------
# Service Account for GitHub Actions
# ----------------------------------------
resource "google_service_account" "github_actions" {
  account_id   = "github-actions-deploy"
  display_name = "GitHub Actions Deploy"
}

resource "google_project_iam_member" "run_admin" {
  project = var.gcp_project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.github_actions.email}"
}

resource "google_project_iam_member" "ar_writer" {
  project = var.gcp_project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.github_actions.email}"
}

resource "google_project_iam_member" "sa_user" {
  project = var.gcp_project_id
  role    = "roles/iam.serviceAccountUser"
  member  = "serviceAccount:${google_service_account.github_actions.email}"
}

resource "google_service_account_key" "github_actions_key" {
  service_account_id = google_service_account.github_actions.name
}

# ----------------------------------------
# Modules
# ----------------------------------------
module "database" {
  source              = "./modules/database"
  neon_api_key        = var.neon_api_key
  existing_project_id = var.neon_project_id
}

module "kafka" {
  source = "./modules/kafka"
}

module "cloudrun" {
  source         = "./modules/cloudrun"
  gcp_project_id = var.gcp_project_id
  gcp_region     = var.gcp_region
  image_prefix   = "${var.gcp_region}-docker.pkg.dev/${var.gcp_project_id}/go-fintech"
  jwt_secret     = var.jwt_secret

  wallet_db_url      = module.database.wallet_db_url
  transaction_db_url = module.database.transaction_db_url
  kafka_brokers      = module.kafka.broker_url
  kafka_username     = module.kafka.username
  kafka_password     = module.kafka.password

  depends_on = [
    google_artifact_registry_repository.fintech,
    module.database,
    module.kafka,
  ]
}
