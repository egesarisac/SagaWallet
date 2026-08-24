terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.1.0"
    }
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.gcp_region
}

locals {
  required_services = toset([
    "artifactregistry.googleapis.com",
    "container.googleapis.com",
    "compute.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "monitoring.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "sts.googleapis.com",
    "vpcaccess.googleapis.com",
  ])

  runtime_service_accounts = {
    auth_api = {
      account_id   = "sagawallet-auth-api"
      display_name = "SagaWallet auth API runtime"
    }
    wallet_api = {
      account_id   = "sagawallet-wallet-api"
      display_name = "SagaWallet wallet API runtime"
    }
    transaction_api = {
      account_id   = "sagawallet-transaction-api"
      display_name = "SagaWallet transaction API runtime"
    }
    wallet_worker = {
      account_id   = "sagawallet-wallet-worker"
      display_name = "SagaWallet wallet worker runtime"
    }
    transaction_worker = {
      account_id   = "sagawallet-transaction-worker"
      display_name = "SagaWallet transaction worker runtime"
    }
    notification_worker = {
      account_id   = "sagawallet-notification-worker"
      display_name = "SagaWallet notification worker runtime"
    }
  }

  runtime_secret_access = {
    auth_api_database = {
      workload = "auth_api"
      secret   = var.secret_names.auth_database_url
    }
    auth_api_jwt = {
      workload = "auth_api"
      secret   = var.secret_names.jwt_secret
    }
    wallet_api_database = {
      workload = "wallet_api"
      secret   = var.secret_names.wallet_database_url
    }
    wallet_api_jwt = {
      workload = "wallet_api"
      secret   = var.secret_names.jwt_secret
    }
    transaction_api_database = {
      workload = "transaction_api"
      secret   = var.secret_names.transaction_database_url
    }
    transaction_api_jwt = {
      workload = "transaction_api"
      secret   = var.secret_names.jwt_secret
    }
    transaction_api_grpc = {
      workload = "transaction_api"
      secret   = var.secret_names.wallet_grpc_token
    }
    wallet_worker_database = {
      workload = "wallet_worker"
      secret   = var.secret_names.wallet_database_url
    }
    wallet_worker_jwt = {
      workload = "wallet_worker"
      secret   = var.secret_names.jwt_secret
    }
    wallet_worker_grpc = {
      workload = "wallet_worker"
      secret   = var.secret_names.wallet_grpc_token
    }
    wallet_worker_kafka_brokers = {
      workload = "wallet_worker"
      secret   = var.secret_names.kafka_brokers
    }
    wallet_worker_kafka_username = {
      workload = "wallet_worker"
      secret   = var.secret_names.kafka_username
    }
    wallet_worker_kafka_password = {
      workload = "wallet_worker"
      secret   = var.secret_names.kafka_password
    }
    transaction_worker_database = {
      workload = "transaction_worker"
      secret   = var.secret_names.transaction_database_url
    }
    transaction_worker_jwt = {
      workload = "transaction_worker"
      secret   = var.secret_names.jwt_secret
    }
    transaction_worker_grpc = {
      workload = "transaction_worker"
      secret   = var.secret_names.wallet_grpc_token
    }
    transaction_worker_kafka_brokers = {
      workload = "transaction_worker"
      secret   = var.secret_names.kafka_brokers
    }
    transaction_worker_kafka_username = {
      workload = "transaction_worker"
      secret   = var.secret_names.kafka_username
    }
    transaction_worker_kafka_password = {
      workload = "transaction_worker"
      secret   = var.secret_names.kafka_password
    }
    notification_worker_kafka_brokers = {
      workload = "notification_worker"
      secret   = var.secret_names.kafka_brokers
    }
    notification_worker_kafka_username = {
      workload = "notification_worker"
      secret   = var.secret_names.kafka_username
    }
    notification_worker_kafka_password = {
      workload = "notification_worker"
      secret   = var.secret_names.kafka_password
    }
  }
}

resource "google_project_service" "required" {
  for_each = local.required_services

  project            = var.gcp_project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "fintech" {
  repository_id = "go-fintech"
  format        = "DOCKER"
  location      = var.gcp_region
  description   = "Immutable SagaWallet service images"

  depends_on = [google_project_service.required]
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
    "roles/run.admin",
  ])

  project = var.gcp_project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.github_actions.email}"
}

resource "google_service_account" "runtime" {
  for_each = local.runtime_service_accounts

  account_id   = each.value.account_id
  display_name = each.value.display_name
}

resource "google_service_account_iam_member" "github_actions_runtime_user" {
  for_each = google_service_account.runtime

  service_account_id = each.value.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_actions.email}"
}

resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "sagawallet-github"
  display_name              = "SagaWallet GitHub Actions"

  depends_on = [google_project_service.required]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github"
  display_name                       = "SagaWallet GitHub repository"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }
  attribute_condition = "assertion.repository == \"${var.github_repository}\" && assertion.ref == \"refs/heads/main\""

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account_iam_member" "github_actions_workload_identity" {
  service_account_id = google_service_account.github_actions.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}

resource "google_secret_manager_secret_iam_member" "runtime_secrets" {
  for_each = local.runtime_secret_access

  project   = var.gcp_project_id
  secret_id = each.value.secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime[each.value.workload].email}"
}

resource "google_compute_network" "saga" {
  name                    = "sagawallet"
  auto_create_subnetworks = false

  depends_on = [google_project_service.required]
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

  gcp_project_id = var.gcp_project_id
  gcp_region     = var.gcp_region
  network        = google_compute_network.saga.id
  subnetwork     = google_compute_subnetwork.workers.id
  worker_service_account_emails = {
    wallet       = google_service_account.runtime["wallet_worker"].email
    transaction  = google_service_account.runtime["transaction_worker"].email
    notification = google_service_account.runtime["notification_worker"].email
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.runtime_secrets,
  ]
}

module "cloudrun" {
  source = "./modules/cloudrun"

  gcp_project_id          = var.gcp_project_id
  gcp_region              = var.gcp_region
  image_prefix            = "${var.gcp_region}-docker.pkg.dev/${var.gcp_project_id}/go-fintech"
  bootstrap_image_digests = var.bootstrap_image_digests
  service_account_emails = {
    wallet      = google_service_account.runtime["wallet_api"].email
    transaction = google_service_account.runtime["transaction_api"].email
    auth        = google_service_account.runtime["auth_api"].email
  }
  secret_names     = var.secret_names
  vpc_connector_id = google_vpc_access_connector.cloud_run.id
  wallet_grpc_addr = "${google_compute_address.wallet_grpc.address}:9081"

  depends_on = [
    google_artifact_registry_repository.fintech,
    google_secret_manager_secret_iam_member.runtime_secrets,
  ]
}

resource "google_monitoring_alert_policy" "consumer_lag" {
  display_name = "SagaWallet Kafka consumer lag"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "Consumer lag remains above threshold"

    condition_prometheus_query_language {
      query               = "max by (group_id, topic) (sagawallet_kafka_consumer_lag) > ${var.consumer_lag_alert_threshold}"
      duration            = "300s"
      evaluation_interval = "60s"
      alert_rule          = "SagaWalletKafkaConsumerLag"
      rule_group          = "sagawallet-workers"
      labels = {
        severity = "warning"
      }
    }
  }

  documentation {
    content   = "Kafka consumer lag has remained above the configured threshold for five minutes. Inspect the affected worker, retry/DLQ rate, and broker health before replaying events."
    mime_type = "text/markdown"
  }

  notification_channels = var.alert_notification_channel_ids
  user_labels = {
    service  = "sagawallet"
    severity = "warning"
  }

  depends_on = [
    google_project_service.required,
    module.gke_workers,
  ]
}
