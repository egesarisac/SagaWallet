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
  description = "Configure GitHub OIDC to impersonate this service account."
  value       = google_service_account.github_actions.email
}

output "gke_cluster_name" {
  value = module.gke_workers.cluster_name
}

output "wallet_grpc_address" {
  description = "Reserved internal address used by the wallet gRPC LoadBalancer service."
  value       = google_compute_address.wallet_grpc.address
}
