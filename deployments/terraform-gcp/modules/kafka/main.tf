terraform {
  required_providers {
    confluent = {
      source  = "confluentinc/confluent"
      version = "~> 2.0"
    }
  }
}

resource "confluent_environment" "fintech" {
  display_name = "go-fintech"
}

resource "confluent_kafka_cluster" "fintech" {
  display_name = "fintech-cluster"
  availability = "SINGLE_ZONE"
  cloud        = "GCP"
  region       = "us-central1"
  basic {}
  environment {
    id = confluent_environment.fintech.id
  }
}

resource "confluent_service_account" "app_manager" {
  display_name = "app-manager"
  description  = "Service account to manage Kafka topics and credentials"
}

resource "confluent_role_binding" "app_manager_cluster_admin" {
  principal   = "User:${confluent_service_account.app_manager.id}"
  role_name   = "CloudClusterAdmin"
  crn_pattern = confluent_kafka_cluster.fintech.rbac_crn
}

resource "confluent_api_key" "app_manager_key" {
  display_name = "app-manager-key"
  description  = "Kafka API Key that is owned by 'app-manager' service account"
  owner {
    id          = confluent_service_account.app_manager.id
    api_version = confluent_service_account.app_manager.api_version
    kind        = confluent_service_account.app_manager.kind
  }
  managed_resource {
    id          = confluent_kafka_cluster.fintech.id
    api_version = confluent_kafka_cluster.fintech.api_version
    kind        = confluent_kafka_cluster.fintech.kind
    environment {
      id = confluent_environment.fintech.id
    }
  }
  depends_on = [
    confluent_role_binding.app_manager_cluster_admin
  ]
}

variable "topics" {
  type = list(string)
  default = [
    "transfer.created",
    "transfer.debit.success",
    "transfer.debit.failed",
    "transfer.credit.success",
    "transfer.credit.failed",
    "transfer.refund.success",
    "transfer.completed",
    "transfer.failed",
    "transfer.dlq"
  ]
}

resource "confluent_kafka_topic" "topics" {
  for_each = toset(var.topics)

  kafka_cluster {
    id = confluent_kafka_cluster.fintech.id
  }
  topic_name       = each.value
  partitions_count = 1
  rest_endpoint    = confluent_kafka_cluster.fintech.rest_endpoint
  credentials {
    key    = confluent_api_key.app_manager_key.id
    secret = confluent_api_key.app_manager_key.secret
  }
}

output "broker_url" {
  value = confluent_kafka_cluster.fintech.bootstrap_endpoint
}

output "username" {
  value = confluent_api_key.app_manager_key.id
}

output "password" {
  value     = confluent_api_key.app_manager_key.secret
  sensitive = true
}
