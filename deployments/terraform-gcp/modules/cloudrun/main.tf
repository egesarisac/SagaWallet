variable "gcp_project_id" {
  type = string
}

variable "gcp_region" {
  type = string
}

variable "image_prefix" {
  type = string
}

variable "runtime_service_account_email" {
  type = string
}

variable "vpc_connector_id" {
  type = string
}

variable "wallet_grpc_addr" {
  type = string
}

variable "secret_names" {
  type = object({
    jwt_secret               = string
    wallet_grpc_token        = string
    wallet_database_url      = string
    transaction_database_url = string
    auth_database_url        = string
    kafka_brokers            = string
    kafka_username           = string
    kafka_password           = string
  })
}

locals {
  api_services = {
    wallet = {
      env = [
        { name = "RUN_KAFKA_CONSUMERS", value = "false" },
        { name = "RUN_GRPC", value = "false" },
        { name = "KAFKA_TLS", value = "true" },
        { name = "LOG_FORMAT", value = "json" },
      ]
      secrets = [
        { name = "DATABASE_URL", secret = var.secret_names.wallet_database_url },
        { name = "JWT_SECRET", secret = var.secret_names.jwt_secret },
        { name = "KAFKA_BROKERS", secret = var.secret_names.kafka_brokers },
        { name = "KAFKA_USERNAME", secret = var.secret_names.kafka_username },
        { name = "KAFKA_PASSWORD", secret = var.secret_names.kafka_password },
        { name = "WALLET_GRPC_TOKEN", secret = var.secret_names.wallet_grpc_token },
      ]
    }
    transaction = {
      env = [
        { name = "RUN_SAGA_WORKERS", value = "false" },
        { name = "WALLET_GRPC_ADDR", value = var.wallet_grpc_addr },
        { name = "KAFKA_TLS", value = "true" },
        { name = "LOG_FORMAT", value = "json" },
      ]
      secrets = [
        { name = "DATABASE_URL", secret = var.secret_names.transaction_database_url },
        { name = "JWT_SECRET", secret = var.secret_names.jwt_secret },
        { name = "KAFKA_BROKERS", secret = var.secret_names.kafka_brokers },
        { name = "KAFKA_USERNAME", secret = var.secret_names.kafka_username },
        { name = "KAFKA_PASSWORD", secret = var.secret_names.kafka_password },
        { name = "WALLET_GRPC_TOKEN", secret = var.secret_names.wallet_grpc_token },
      ]
    }
    auth = {
      env = [
        { name = "ACCESS_TOKEN_EXPIRY_MINUTES", value = "15" },
        { name = "JWT_ISSUER", value = "sagawallet-auth" },
        { name = "LOG_FORMAT", value = "json" },
      ]
      secrets = [
        { name = "DATABASE_URL", secret = var.secret_names.auth_database_url },
        { name = "JWT_SECRET", secret = var.secret_names.jwt_secret },
      ]
    }
  }
}

resource "google_cloud_run_v2_service" "api" {
  for_each = local.api_services

  name     = "${each.key}-service"
  location = var.gcp_region
  ingress  = "INGRESS_TRAFFIC_ALL"

  lifecycle {
    # CD deploys a verified immutable digest; Terraform owns the runtime shape.
    ignore_changes = [template[0].containers[0].image]
  }

  template {
    service_account = var.runtime_service_account_email

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    vpc_access {
      connector = var.vpc_connector_id
      egress    = "ALL_TRAFFIC"
    }

    containers {
      image = "${var.image_prefix}/${each.key}-service:latest"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = each.value.env
        content {
          name  = env.value.name
          value = env.value.value
        }
      }

      dynamic "env" {
        for_each = each.value.secrets
        content {
          name = env.value.name
          value_source {
            secret_key_ref {
              secret  = env.value.secret
              version = "latest"
            }
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  for_each = google_cloud_run_v2_service.api

  location = each.value.location
  name     = each.value.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

output "wallet_service_url" {
  value = google_cloud_run_v2_service.api["wallet"].uri
}

output "transaction_service_url" {
  value = google_cloud_run_v2_service.api["transaction"].uri
}

output "auth_service_url" {
  value = google_cloud_run_v2_service.api["auth"].uri
}
