variable "gcp_project_id" { type = string }
variable "gcp_region" { type = string }
variable "image_prefix" { type = string }
variable "jwt_secret" {
  type      = string
  sensitive = true
}
variable "wallet_grpc_token" {
  type      = string
  sensitive = true
}
variable "wallet_db_url" {
  type      = string
  sensitive = true
}
variable "transaction_db_url" {
  type      = string
  sensitive = true
}
variable "auth_db_url" {
  type      = string
  sensitive = true
}
variable "kafka_brokers" {
  type      = string
  sensitive = true
}
variable "kafka_username" {
  type      = string
  sensitive = true
}
variable "kafka_password" {
  type      = string
  sensitive = true
}

locals {
  kafka_env = [
    { name = "KAFKA_BROKERS", value = var.kafka_brokers },
    { name = "KAFKA_USERNAME", value = var.kafka_username },
    { name = "KAFKA_PASSWORD", value = var.kafka_password },
    { name = "KAFKA_TLS", value = "true" },
  ]
}

# ----------------------------------------
# wallet-service
# ----------------------------------------
resource "google_cloud_run_v2_service" "wallet" {
  name     = "wallet-service"
  location = var.gcp_region
  ingress  = "INGRESS_TRAFFIC_ALL"

  lifecycle {
    # CI/CD owns the deployed image. Terraform manages infra + config.
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }
    containers {
      image = "${var.image_prefix}/wallet-service:latest"
      ports { container_port = 8080 }

      env {
        name  = "HTTP_PORT"
        value = "8080"
      }
      env {
        name  = "GRPC_PORT"
        value = "9081"
      }
      env {
        name  = "DATABASE_URL"
        value = var.wallet_db_url
      }
      env {
        name  = "DB_SSLMODE"
        value = "require"
      }
      env {
        name  = "JWT_SECRET"
        value = var.jwt_secret
      }
      env {
        name  = "WALLET_GRPC_TOKEN"
        value = var.wallet_grpc_token
      }
      env {
        name  = "LOG_FORMAT"
        value = "json"
      }
      env {
        name  = "LOG_LEVEL"
        value = "info"
      }
      env {
        name  = "DISABLE_RATE_LIMIT"
        value = "false"
      }

      env {
        name  = "SKIP_MIGRATIONS"
        value = "true"
      }

      dynamic "env" {
        for_each = local.kafka_env
        content {
          name  = env.value.name
          value = env.value.value
        }
      }

      resources {
        limits = { cpu = "1", memory = "512Mi" }
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "wallet_public" {
  location = google_cloud_run_v2_service.wallet.location
  name     = google_cloud_run_v2_service.wallet.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# ----------------------------------------
# transaction-service
# ----------------------------------------
resource "google_cloud_run_v2_service" "transaction" {
  name     = "transaction-service"
  location = var.gcp_region
  ingress  = "INGRESS_TRAFFIC_ALL"

  lifecycle {
    # CI/CD owns the deployed image (Option A). Terraform manages infra + config.
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }
    containers {
      image = "${var.image_prefix}/transaction-service:latest"
      ports { container_port = 8080 }

      env {
        name  = "HTTP_PORT"
        value = "8080"
      }
      env {
        name  = "GRPC_PORT"
        value = "9082"
      }
      env {
        name  = "DATABASE_URL"
        value = var.transaction_db_url
      }
      env {
        name  = "DB_SSLMODE"
        value = "require"
      }
      env {
        name  = "JWT_SECRET"
        value = var.jwt_secret
      }
      env {
        name  = "WALLET_GRPC_TOKEN"
        value = var.wallet_grpc_token
      }
      env {
        name  = "WALLET_SERVICE_URL"
        value = google_cloud_run_v2_service.wallet.uri
      }
      env {
        name  = "LOG_FORMAT"
        value = "json"
      }
      env {
        name  = "LOG_LEVEL"
        value = "info"
      }
      env {
        name  = "DISABLE_RATE_LIMIT"
        value = "false"
      }

      env {
        name  = "SKIP_MIGRATIONS"
        value = "true"
      }

      dynamic "env" {
        for_each = local.kafka_env
        content {
          name  = env.value.name
          value = env.value.value
        }
      }

      resources {
        limits = { cpu = "1", memory = "512Mi" }
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "transaction_public" {
  location = google_cloud_run_v2_service.transaction.location
  name     = google_cloud_run_v2_service.transaction.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# ----------------------------------------
# notification-service
# ----------------------------------------
resource "google_cloud_run_v2_service" "notification" {
  name     = "notification-service"
  location = var.gcp_region
  ingress  = "INGRESS_TRAFFIC_ALL"

  lifecycle {
    # CI/CD owns the deployed image (Option A). Terraform manages infra + config.
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }
    containers {
      image = "${var.image_prefix}/notification-service:latest"
      ports { container_port = 8080 }

      env {
        name  = "HTTP_PORT"
        value = "8080"
      }
      env {
        name  = "LOG_FORMAT"
        value = "json"
      }
      env {
        name  = "LOG_LEVEL"
        value = "info"
      }

      dynamic "env" {
        for_each = local.kafka_env
        content {
          name  = env.value.name
          value = env.value.value
        }
      }

      resources {
        limits = { cpu = "1", memory = "512Mi" }
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "notification_public" {
  location = google_cloud_run_v2_service.notification.location
  name     = google_cloud_run_v2_service.notification.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# ----------------------------------------
# auth-service
# ----------------------------------------
resource "google_cloud_run_v2_service" "auth" {
  name     = "auth-service"
  location = var.gcp_region
  ingress  = "INGRESS_TRAFFIC_ALL"

  lifecycle {
    # CI/CD owns the deployed image (Option A). Terraform manages infra + config.
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }

    containers {
      image = "${var.image_prefix}/auth-service:latest"
      ports { container_port = 8080 }

      env {
        name  = "HTTP_PORT"
        value = "8080"
      }
      env {
        name  = "DATABASE_URL"
        value = var.auth_db_url
      }
      env {
        name  = "DB_SSLMODE"
        value = "require"
      }
      env {
        name  = "JWT_SECRET"
        value = var.jwt_secret
      }
      env {
        name  = "ACCESS_TOKEN_EXPIRY_MINUTES"
        value = "15"
      }
      env {
        name  = "REFRESH_TOKEN_EXPIRY_HOURS"
        value = "168"
      }
      env {
        name  = "JWT_ISSUER"
        value = "sagawallet-auth"
      }
      env {
        name  = "LOG_FORMAT"
        value = "json"
      }
      env {
        name  = "LOG_LEVEL"
        value = "info"
      }
      env {
        name  = "DISABLE_RATE_LIMIT"
        value = "false"
      }

      resources {
        limits = { cpu = "1", memory = "512Mi" }
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "auth_public" {
  location = google_cloud_run_v2_service.auth.location
  name     = google_cloud_run_v2_service.auth.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

output "wallet_service_url" {
  value = google_cloud_run_v2_service.wallet.uri
}
output "transaction_service_url" {
  value = google_cloud_run_v2_service.transaction.uri
}
output "notification_service_url" {
  value = google_cloud_run_v2_service.notification.uri
}
output "auth_service_url" {
  value = google_cloud_run_v2_service.auth.uri
}
