# Contributing to SagaWallet

This project follows an industry-standard Pull Request (PR)-first workflow.

## Branching Model

- Protected branch: `main`
- Working branches: short-lived feature/fix/chore branches created from `main`

Recommended branch names:

- `feat/<scope>-<short-description>`
- `fix/<scope>-<short-description>`
- `chore/<scope>-<short-description>`
- `docs/<scope>-<short-description>`

## Required Git Process

1. Sync with latest `main`.
2. Create a new branch from `main`.
3. Commit your changes to that branch.
4. Push branch and open a PR targeting `main`.
5. Wait for CI and address review feedback.
6. Merge only after all required checks pass.

Example:

```bash
git checkout main
git pull origin main
git checkout -b feat/wallet-add-balance-filter
git push -u origin feat/wallet-add-balance-filter
```

## CI/CD Behavior

- On PR to `main`: run lint and tests.
- On push to `main` (after merge): build images and deploy.

## Merge Strategy

- Prefer **Squash and merge** to keep history readable.
- Delete branch after merge.

## Commit Message Guidance

Use conventional-style prefixes when possible:

- `feat:` new functionality
- `fix:` bug fix
- `chore:` maintenance
- `docs:` documentation
- `refactor:` code restructuring
- `test:` test changes
