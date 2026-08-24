# GCP Deployment

This is the supported production-shaped deployment path for SagaWallet:

- public auth, wallet, and transaction APIs run on Cloud Run;
- Kafka consumers, the outbox publisher, timeout processing, and wallet gRPC
  run continuously on GKE Autopilot;
- Secret Manager values are mounted or injected with workload-specific IAM;
- GitHub Actions authenticates through Workload Identity Federation, without a
  service-account JSON key;
- Cloud Run starts from immutable image digests and CD promotes only validated
  commit digests.

Before `terraform apply`, create the Secret Manager secrets listed in
`variables.tf`, build the three API images, and provide their
`sha256:<digest>` values through `bootstrap_image_digests`. Also provide the
GitHub repository in `owner/name` form and at least one existing Cloud
Monitoring notification channel resource name.

After apply, configure these GitHub repository variables from Terraform output:

```text
GCP_DEPLOY_SERVICE_ACCOUNT      = github_actions_service_account
GCP_WORKLOAD_IDENTITY_PROVIDER  = github_workload_identity_provider
```

The Workload Identity provider accepts only the configured repository's
`main` branch. Runtime service accounts are separate for every API and worker,
and each account receives Secret Manager access only for the values used by
that workload.
