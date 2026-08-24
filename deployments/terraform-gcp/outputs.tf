output "wallet_service_url" {
  value = module.cloudrun.wallet_service_url
}

output "transaction_service_url" {
  value = module.cloudrun.transaction_service_url
}

output "auth_service_url" {
  value = module.cloudrun.auth_service_url
}

output "artifact_registry" {
  value = "${var.gcp_region}-docker.pkg.dev/${var.gcp_project_id}/go-fintech"
}

output "github_actions_service_account" {
  description = "Set this as the GCP_DEPLOY_SERVICE_ACCOUNT GitHub repository variable."
  value       = google_service_account.github_actions.email
}

output "github_workload_identity_provider" {
  description = "Set this as the GCP_WORKLOAD_IDENTITY_PROVIDER GitHub repository variable."
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "runtime_service_accounts" {
  description = "Workload-specific API and worker service accounts."
  value = {
    for name, account in google_service_account.runtime : name => account.email
  }
}

output "gke_cluster_name" {
  value = module.gke_workers.cluster_name
}

output "wallet_grpc_address" {
  description = "Reserved internal address used by the wallet gRPC LoadBalancer service."
  value       = google_compute_address.wallet_grpc.address
}

output "consumer_lag_alert_policy" {
  value = google_monitoring_alert_policy.consumer_lag.name
}
