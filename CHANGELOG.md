# Changelog

## 1.0.0 (Unreleased)

### Features

- AI Chat Assistant plugin for Grafana with LLM-powered conversations
- Chat history with persistent storage and full-text search
- MCP (Model Context Protocol) integration for tool use
- Token counting and conversation management
- Configurable API settings (Azure OpenAI, custom endpoints)

### CI/CD

- CI pipeline: frontend lint, typecheck, unit tests, build + backend vet, test, build
- Multi-platform cross-compilation (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64)
- E2E tests with Playwright against Grafana 12.1.0
- Release workflow with multi-platform builds and placeholder plugin signing

### Maintenance

- Standardized plugin org to `grafana` across plugin.json and provisioning
- Replaced `lma`/`collab` references with `Cisco Systems, Inc.`
