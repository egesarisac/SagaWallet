terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.gcp_region
}

resource "google_artifact_registry_repository" "fintech" {
  repository_id = "go-fintech"
  format        = "DOCKER"
  location      = var.gcp_region
  description   = "Immutable SagaWallet service images"
}

resource "google_service_account" "github_actions" {
  account_id   = "github-actions-deploy"
  display_name = "SagaWallet GitHub Actions deployer"
}

resource "google_project_iam_member" "github_actions_roles" {
  for_each = toset([
    "roles/artifactregistry.writer",
    "roles/container.developer",
    "roles/compute.viewer",
    "roles/iam.serviceAccountUser",
    "roles/run.admin",
  ])

  project = var.gcp_project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.github_actions.email}"
}

resource "google_service_account" "runtime" {
  account_id   = "sagawallet-runtime"
  display_name = "SagaWallet API and worker runtime"
}

resource "google_secret_manager_secret_iam_member" "runtime_secrets" {
  for_each = toset(values(var.secret_names))

  project   = var.gcp_project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_compute_network" "saga" {
  name                    = "sagawallet"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "workers" {
  name          = "sagawallet-workers"
  ip_cidr_range = var.worker_subnet_cidr
  region        = var.gcp_region
  network       = google_compute_network.saga.id

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = var.pod_subnet_cidr
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = var.service_subnet_cidr
  }
}

resource "google_compute_router" "saga" {
  name    = "sagawallet-router"
  region  = var.gcp_region
  network = google_compute_network.saga.id
}

resource "google_compute_router_nat" "saga" {
  name                               = "sagawallet-nat"
  router                             = google_compute_router.saga.name
  region                             = var.gcp_region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

resource "google_vpc_access_connector" "cloud_run" {
  name          = "sagawallet-cloud-run"
  region        = var.gcp_region
  network       = google_compute_network.saga.name
  ip_cidr_range = var.cloud_run_connector_cidr
  min_instances = 2
  max_instances = 3
}

resource "google_compute_address" "wallet_grpc" {
  name         = "sagawallet-wallet-grpc"
  address_type = "INTERNAL"
  region       = var.gcp_region
  subnetwork   = google_compute_subnetwork.workers.id
}

module "gke_workers" {
  source = "./modules/gke"

  gcp_project_id                = var.gcp_project_id
  gcp_region                    = var.gcp_region
  network                       = google_compute_network.saga.id
  subnetwork                    = google_compute_subnetwork.workers.id
  runtime_service_account_email = google_service_account.runtime.email
}

module "cloudrun" {
  source = "./modules/cloudrun"

  gcp_project_id                = var.gcp_project_id
  gcp_region                    = var.gcp_region
  image_prefix                  = "${var.gcp_region}-docker.pkg.dev/${var.gcp_project_id}/go-fintech"
  runtime_service_account_email = google_service_account.runtime.email
  secret_names                  = var.secret_names
  vpc_connector_id              = google_vpc_access_connector.cloud_run.id
  wallet_grpc_addr              = "${google_compute_address.wallet_grpc.address}:9081"

  depends_on = [
    google_artifact_registry_repository.fintech,
    google_secret_manager_secret_iam_member.runtime_secrets,
  ]
}
