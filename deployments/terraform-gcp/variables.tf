variable "gcp_project_id" {
  description = "GCP project that owns SagaWallet infrastructure."
  type        = string
}

variable "gcp_region" {
  description = "Region for Cloud Run, GKE Autopilot, and Artifact Registry."
  type        = string
  default     = "us-central1"
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
