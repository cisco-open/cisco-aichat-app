# Changelog

## 1.1.0 (2026-05-11)

### Security

- Resolved CodeQL alerts: removed database URLs from error messages to prevent credential leakage
- Replaced `Math.random()` with `crypto.randomUUID()` for session ID generation
- Added least-privilege `permissions` to CI and E2E workflows

### Maintenance

- Set up Dependabot for automated grouped dependency updates (npm, Go modules, GitHub Actions) with 14-day cooldown
- Enabled CodeQL default setup and GitHub secret scanning with push protection
- Bumped GitHub Actions: checkout v6, setup-node v6, setup-go v6, upload-artifact v7, action-gh-release v3
- Bumped anthropic-sdk-go v1.20.0 → v1.38.0 (with string literal fix for removed constants)
- Applied 14 npm security dependency patches
- Added CLAUDE.md for AI-assisted development

## 1.0.0 (2026-05-11)

### Features

- AI Chat Assistant plugin for Grafana with LLM-powered conversations
- Chat history with persistent storage (file, SQLite, PostgreSQL) and full-text search
- MCP (Model Context Protocol) integration for tool use
- Token counting and conversation management
- Configurable API settings (Azure OpenAI, custom endpoints)

### CI/CD

- CI pipeline: frontend lint, typecheck, unit tests, build + backend vet, test, build
- Multi-platform cross-compilation (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64)
- E2E tests with Playwright against Grafana 12.1.0
- Release workflow with multi-platform builds and placeholder plugin signing

### Maintenance

- Standardized plugin org to `cisco` across plugin.json and provisioning
- Replaced `lma`/`collab` references with `Cisco Systems, Inc.`
