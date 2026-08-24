variable "gcp_project_id" {
  type = string
}

variable "gcp_region" {
  type = string
}

variable "network" {
  type = string
}

variable "subnetwork" {
  type = string
}

variable "worker_service_account_emails" {
  type = object({
    wallet       = string
    transaction  = string
    notification = string
  })
}

resource "google_container_cluster" "workers" {
  name             = "sagawallet-workers"
  location         = var.gcp_region
  enable_autopilot = true

  network    = var.network
  subnetwork = var.subnetwork

  release_channel {
    channel = "REGULAR"
  }

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  workload_identity_config {
    workload_pool = "${var.gcp_project_id}.svc.id.goog"
  }

  secret_manager_config {
    enabled = true
  }

  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]

    managed_prometheus {
      enabled = true
    }
  }
}

resource "google_service_account_iam_member" "worker_workload_identity" {
  for_each = var.worker_service_account_emails

  service_account_id = "projects/${var.gcp_project_id}/serviceAccounts/${each.value}"
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.gcp_project_id}.svc.id.goog[saga-workers/${each.key}-worker]"
}

output "cluster_name" {
  value = google_container_cluster.workers.name
}

output "worker_service_account_emails" {
  value = var.worker_service_account_emails
}
