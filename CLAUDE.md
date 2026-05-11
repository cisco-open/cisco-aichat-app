# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Grafana AI Chat Assistant plugin — a Grafana app plugin with a Go backend and React/TypeScript frontend. Provides an intelligent chat interface with LLM integration (Anthropic, OpenAI, Azure OpenAI) and MCP (Model Context Protocol) tool support.

## Build & Development Commands

### Frontend
```bash
npm run dev          # Webpack watch mode
npm run build        # Production build
npm run lint         # ESLint
npm run lint:fix     # ESLint + Prettier auto-fix
npm run typecheck    # tsc --noEmit
```

### Backend
```bash
go build -v ./pkg/...         # Build all packages
go vet ./...                  # Vet
mage build:buildAll           # Full plugin build (uses Magefile.go)
```

### Tests
```bash
# Frontend unit tests
npm run test              # Jest watch mode
npm run test:ci           # Jest CI mode (maxWorkers 4)
npx jest path/to/file     # Single test file

# Backend tests
go test -v -race ./...              # All with race detector
go test -v -race ./pkg/storage/     # Single package

# E2E
npm run e2e               # Playwright (requires Grafana running)
npx playwright test tests/specific.spec.ts  # Single E2E test
```

### Local Development
```bash
docker compose up --build   # Start Grafana with plugin mounted (port 3000)
npm run dev                 # In separate terminal for frontend hot reload
```

## Architecture

### Backend (pkg/)

Go plugin built on `grafana-plugin-sdk-go`. The main app struct in `pkg/plugin/app.go` implements `CallResourceHandler` for REST API endpoints.

Key packages:
- `pkg/plugin/` — HTTP resource handlers, rate limiting (10 req/sec per user), input validation
- `pkg/storage/` — Session persistence with factory pattern: auto-detects PostgreSQL/SQLite from env vars, falls back to file or memory storage. Uses `golang-migrate` for schema migrations.
- `pkg/cache/` — Message and session caching layer (ristretto)
- `pkg/tokens/` — Token counting per LLM provider, context window management, auto-compaction
- `pkg/context/` — Context assembly for LLM requests
- `pkg/metrics/` — Prometheus instrumentation
- `pkg/telemetry/` — OpenTelemetry tracing

Storage resolution order: `AICHAT_DATABASE_URL` → `GF_DATABASE_URL` → `GF_DATABASE_*` components → `grafana.ini` → file storage → memory fallback.

### Frontend (src/)

React 18 + TypeScript app registered as a Grafana AppPlugin in `src/module.tsx`.

Key layers:
- `src/components/` — UI components (Chat*, InfiniteMessageList, PermissionGuard, ToolExecutionIndicator)
- `src/services/` — Backend communication (`ChatBackendService`), storage (`ChatStorageService`), MCP integration (`MCPIntegrationService`), RBAC (`PermissionService`)
- `src/hooks/` — Custom React hooks
- `src/context/` — React context providers

Streaming responses use RxJS. MCP tool calls are rendered with `ToolExecutionIndicator`.

### Plugin Communication

Frontend calls backend via Grafana's plugin proxy (`/api/plugins/cisco-aichat-app/resources/...`). The backend forwards to configured LLM providers and manages session state.

## CI/CD

- **CI** (`.github/workflows/ci.yml`): Frontend lint/typecheck/test/build + Backend vet/test + multiplatform cross-compilation
- **E2E** (`.github/workflows/e2e.yml`): Playwright tests against Grafana in Docker
- **Release** (`.github/workflows/release.yml`): Triggered by `v*` tags, builds and creates GitHub release
- **Dependabot**: Weekly grouped updates (npm, gomod, github-actions) with 14-day cooldown

## Documentation

When making changes, update the relevant docs in the same PR:
- `CHANGELOG.md` — new features, fixes, and maintenance (grouped under the next version heading)
- `README.md` — user-facing feature descriptions, configuration, and architecture
- `SECURITY.md` — security tooling or vulnerability process changes
- `docs/API.md` — backend endpoint additions or modifications

## Environment Variables

The backend reads LLM configuration from Grafana's secure JSON data (plugin settings), not from env vars directly. Storage config uses:
- `AICHAT_DATABASE_URL` — explicit DB override
- `GF_DATABASE_URL` — Grafana's unified DB URL
- `GF_DATABASE_TYPE`, `GF_DATABASE_HOST`, `GF_DATABASE_NAME`, `GF_DATABASE_USER`, `GF_DATABASE_PASSWORD` — component-based config
