variable "gcp_project_id" {
  description = "GCP project ID"
  type        = string
}

variable "gcp_region" {
  description = "GCP region for Cloud Run and Artifact Registry"
  type        = string
  default     = "us-central1"
}

variable "confluent_cloud_api_key" {
  description = "Confluent Cloud API key"
  type        = string
  sensitive   = true
}

variable "confluent_cloud_api_secret" {
  description = "Confluent Cloud API secret"
  type        = string
  sensitive   = true
}

variable "neon_project_id" {
  description = "Neon Project ID (the slug from the dashboard URL)"
  type        = string
}

variable "neon_api_key" {
  description = "Neon API key for serverless PostgreSQL"
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT signing secret shared across services"
  type        = string
  sensitive   = true
}

variable "wallet_grpc_token" {
  description = "Required internal service token for wallet<->transaction gRPC authentication"
  type        = string
  sensitive   = true
}
