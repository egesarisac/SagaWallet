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

variable "runtime_service_account_email" {
  type = string
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

}

resource "google_service_account_iam_member" "worker_workload_identity" {
  service_account_id = "projects/${var.gcp_project_id}/serviceAccounts/${var.runtime_service_account_email}"
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.gcp_project_id}.svc.id.goog[saga-workers/saga-worker]"
}

output "cluster_name" {
  value = google_container_cluster.workers.name
}
