# SagaWorker Deployments

`workers.yaml` describes the always-on GKE Autopilot workloads. Cloud Run owns
the public `auth-service`, `wallet-service`, and `transaction-service` APIs;
these deployments own Kafka consumption, the transactional outbox publisher,
the timeout/DLQ workers, and wallet's internal gRPC server.

Terraform intentionally receives only Secret Manager secret IDs. Before any
apply, create the secret versions named in `terraform-gcp/variables.tf` and
configure GitHub OIDC to impersonate the Terraform output service account.
Enable the GKE Secret Manager add-on before applying the worker manifest; it
supplies the `secrets-store-gke.csi.k8s.io` CSI driver used to mount secret
files into the pods.

The CD workflow renders this file with immutable image digests, the reserved
wallet gRPC address, the GCP project ID, and the workload identity service
account. It applies no secret values to Kubernetes.
