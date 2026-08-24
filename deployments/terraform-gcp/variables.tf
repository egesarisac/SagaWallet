variable "gcp_project_id" {
  description = "GCP project that owns SagaWallet infrastructure."
  type        = string
}

variable "gcp_region" {
  description = "Region for Cloud Run, GKE Autopilot, and Artifact Registry."
  type        = string
  default     = "us-central1"
}

variable "github_repository" {
  description = "GitHub repository allowed to deploy from main through Workload Identity Federation, in owner/name form."
  type        = string

  validation {
    condition     = can(regex("^[^/]+/[^/]+$", var.github_repository))
    error_message = "github_repository must use owner/name form."
  }
}

variable "bootstrap_image_digests" {
  description = "Initial immutable Artifact Registry digests used when Terraform first creates the Cloud Run services."
  type = object({
    auth        = string
    wallet      = string
    transaction = string
  })

  validation {
    condition = alltrue([
      for digest in values(var.bootstrap_image_digests) : can(regex("^sha256:[0-9a-f]{64}$", digest))
    ])
    error_message = "Every bootstrap image digest must be a lowercase sha256:<64 hex characters> reference."
  }
}

variable "alert_notification_channel_ids" {
  description = "Cloud Monitoring notification channel resource names that receive consumer-lag incidents."
  type        = list(string)

  validation {
    condition     = length(var.alert_notification_channel_ids) > 0
    error_message = "At least one alert notification channel is required."
  }
}

variable "consumer_lag_alert_threshold" {
  description = "Maximum Kafka consumer lag tolerated for five minutes before alerting."
  type        = number
  default     = 100

  validation {
    condition     = var.consumer_lag_alert_threshold > 0
    error_message = "consumer_lag_alert_threshold must be greater than zero."
  }
}

variable "worker_subnet_cidr" {
  type    = string
  default = "10.20.0.0/20"
}

variable "pod_subnet_cidr" {
  type    = string
  default = "10.24.0.0/16"
}

variable "service_subnet_cidr" {
  type    = string
  default = "10.25.0.0/20"
}

variable "cloud_run_connector_cidr" {
  type    = string
  default = "10.8.0.0/28"
}

variable "secret_names" {
  description = "Existing Secret Manager secret IDs. Terraform never receives secret payloads."
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
  default = {
    jwt_secret               = "sagawallet-jwt"
    wallet_grpc_token        = "sagawallet-wallet-grpc-token"
    wallet_database_url      = "sagawallet-wallet-database-url"
    transaction_database_url = "sagawallet-transaction-database-url"
    auth_database_url        = "sagawallet-auth-database-url"
    kafka_brokers            = "sagawallet-kafka-brokers"
    kafka_username           = "sagawallet-kafka-username"
    kafka_password           = "sagawallet-kafka-password"
  }
}
