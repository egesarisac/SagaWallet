# SagaWorker Deployments

`workers.yaml` describes the always-on GKE Autopilot workloads. Cloud Run owns
the public `auth-service`, `wallet-service`, and `transaction-service` APIs;
these deployments own Kafka consumption, the transactional outbox publisher,
the timeout/DLQ workers, and wallet's internal gRPC server.

Terraform intentionally receives only Secret Manager secret IDs. Before any
apply, create the secret versions named in `terraform-gcp/variables.tf` and
configure the GitHub repository variables from the Terraform Workload Identity
Federation outputs. Terraform enables the GKE Secret Manager and Managed
Prometheus add-ons used by this manifest.

The CD workflow renders this file with immutable image digests, the reserved
wallet gRPC address, the GCP project ID, and three workload-specific service
accounts. Each worker can mount only its own SecretProviderClass; the
notification worker, for example, has no database or JWT access.

`PodMonitoring` scrapes each worker's `/metrics` endpoint. The shared Kafka
library exports `sagawallet_kafka_consumer_lag`, and Terraform installs a Cloud
Monitoring alert that fires when a consumer group remains above the configured
lag threshold for five minutes.
