# Changelog

## 1.6.0

- Add authenticated load balancer creation and detail tools. Select an available
  reserved Floating IP during creation and read its allocation, VIP and regional
  status. Config, permissions and billing remain enforced by the public API.
- Use stable SDK v0.17.0, preserving NAT IP retention and allocation guards.
- Verify real HTTP responses before and after recreating an LB on the same IP,
  including busy-IP release and double-claim protection.

## 1.5.0

- Add NAT gateway disable with explicit public IP retention and an optional
  allocation guard; enable NAT using a retained address or a newly allocated IP.
  Tool descriptions explain asynchronous attachment, connectivity and billing.
- Include authenticated Git app branch updates from the current main branch.
- Verify the NAT retention and reuse cycle against the production API using
  a local MCP server and the CLI; preserve API authorization and conflict checks.

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
