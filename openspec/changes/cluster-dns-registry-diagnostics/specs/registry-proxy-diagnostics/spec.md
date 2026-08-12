## ADDED Requirements

### Requirement: Diagnose Registry Proxy egress from the actual proxy workload

The system SHALL run a fixed, bounded diagnostic from a Ready Pod of a selected platform-managed Registry Proxy and SHALL distinguish Kubernetes readiness from upstream egress health.

#### Scenario: Diagnose healthy Registry egress

- **WHEN** an administrator requests diagnosis for a Ready managed Registry Proxy and its configured upstream returns a valid Registry `/v2/` response
- **THEN** the system SHALL report `healthy` with bounded resolver, address, HTTP class, and elapsed-time evidence
- **AND THEN** it SHALL not expose credentials, authorization headers, proxy URLs, environment values, or arbitrary command output

#### Scenario: Diagnose unreachable upstream address

- **WHEN** the selected Proxy Pod resolves its configured upstream but the TCP connection does not complete before the fixed deadline
- **THEN** the system SHALL report `upstream_connect_timeout`
- **AND THEN** it SHALL identify the tested address and elapsed time in the structured response

#### Scenario: Proxy has no Ready workload

- **WHEN** an administrator requests diagnosis for a managed Proxy with no Ready Pod
- **THEN** the system SHALL return `proxy_not_ready`
- **AND THEN** it SHALL not execute a diagnostic in another Pod or on a node

#### Scenario: Reject arbitrary network targets

- **WHEN** a request contains a URL, image, Pod name, shell command, or Registry not bound to a managed Proxy
- **THEN** the system SHALL reject the request before selecting a Pod or making a network probe

### Requirement: Configure Registry Proxy outbound proxies safely

The system SHALL allow an administrator to configure optional HTTP, HTTPS, and no-proxy settings for a managed Registry Proxy without exposing credential-bearing values.

#### Scenario: Save outbound proxy settings

- **WHEN** an administrator saves valid HTTP or HTTPS outbound proxy settings for a managed Proxy
- **THEN** the system SHALL encrypt sensitive values at rest and inject only the corresponding environment variables into that Proxy Deployment
- **AND THEN** it SHALL perform a controlled Proxy rollout

#### Scenario: Read configured outbound proxy state

- **WHEN** the Registry Mirrors page reads a managed Proxy configuration
- **THEN** the system SHALL expose whether an outbound proxy is configured and the non-sensitive no-proxy value only
- **AND THEN** it SHALL NOT return proxy URL credentials, tokens, or passwords

### Requirement: Agent receives fixed read-only DNS and Proxy evidence

The system SHALL expose only allowlisted DNS status/resolution and configured Registry Proxy diagnostics to authorized Runtime Agents.

#### Scenario: Agent diagnoses a configured Registry Proxy

- **WHEN** an authenticated Runtime with `registry.proxy_diagnose` requests a configured Registry
- **THEN** the system SHALL run the same bounded managed-Proxy diagnostic used by the browser API
- **AND THEN** it SHALL return the structured classification without granting arbitrary network access

#### Scenario: Agent resolves an allowed domain

- **WHEN** an authenticated Runtime with `dns.read` requests an allowlisted domain
- **THEN** the system SHALL return bounded CoreDNS consistency evidence for that domain
- **AND THEN** it SHALL reject any domain outside the Manager-maintained allowlist
