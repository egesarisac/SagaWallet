output "wallet_service_url" {
  description = "Cloud Run URL for wallet-service"
  value       = module.cloudrun.wallet_service_url
}

output "transaction_service_url" {
  description = "Cloud Run URL for transaction-service"
  value       = module.cloudrun.transaction_service_url
}

output "notification_service_url" {
  description = "Cloud Run URL for notification-service"
  value       = module.cloudrun.notification_service_url
}

output "auth_service_url" {
  description = "Cloud Run URL for auth-service"
  value       = module.cloudrun.auth_service_url
}

output "artifact_registry" {
  description = "Artifact Registry hostname for Docker pushes"
  value       = "${var.gcp_region}-docker.pkg.dev/${var.gcp_project_id}/go-fintech"
}

output "github_actions_sa_key" {
  description = "Raw JSON service account key — add this as GCP_SA_KEY GitHub secret"
  value       = base64decode(google_service_account_key.github_actions_key.private_key)
  sensitive   = true
}

output "kafka_brokers" {
  value     = module.kafka.broker_url
  sensitive = true
}

output "kafka_username" {
  value     = module.kafka.username
  sensitive = true
}

output "kafka_password" {
  value     = module.kafka.password
  sensitive = true
}

output "wallet_db_url" {
  value     = module.database.wallet_db_url
  sensitive = true
}

output "transaction_db_url" {
  value     = module.database.transaction_db_url
  sensitive = true
}

output "auth_db_url" {
  value     = module.database.auth_db_url
  sensitive = true
}
