# Changelog

## 1.4.4 — unreleased

- Update the tool catalog, server instructions and Registry description to include
  Kafka, VM creation and backups, regional S3, VPC renaming and AI model discovery.
- Document OAuth configuration, deployment verification and the separate Git tag,
  GitHub Release, production rollout and Registry publication steps.
- Share authenticated JSON transport across S3, Kafka, VMs and VPC tools. Preserve
  API key/OAuth delegation; use actionable API errors and handle successful 204
  responses without reporting a JSON failure. Reject oversized responses explicitly.
- Keep release-tag image builds from overwriting the production SHA image tag.
- Clarify destructive-tool confirmation and the one-time Kafka password exception.

## 1.4.3

- Remove prepaid VM backup slots from the tool contract. Backup storage is billed
  separately by actual compressed usage; disabling schedules preserves copies.

## 1.4.2

- Add guarded deletion of VM recovery points through the public API.

## 1.4.1

- Create VMs with an optional backup policy saved atomically. List recovery point
  origin, expiry, compressed size and legacy immutable protection deadlines.

## 1.4.0

- Add VPC display-name changes, regional S3 discovery/bucket creation and VM backup
  policy tools. The experimental Kubernetes TLS tool was removed before this tag.

## 1.3.0

- Add paid Kafka resume and expose API hourly billing quotes.

## 1.2.0

- Add managed Kafka Dev/HA lifecycle tools and a restricted node-power canary tool.
- Add public AI chat/image/video model discovery and correct its MCP output schema.
- Improve connection/deployment guides and update CI Actions.

## 1.1.0

- Search apps by exact repository, branch, name and domain across API pages.
- Find builds by full commit SHA and distinguish successful builds from deployments
  that have reached `deploy_state=live`.

Registry-only metadata publication `1.1.1` is separate from these Git tags.
