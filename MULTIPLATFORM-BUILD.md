# Multi-Platform Build Guide

This document explains the build options available for the Grafana AI Chat App plugin, which includes both frontend TypeScript/React components and a Go backend for chat history persistence.

## Build Methods

### Option 1: Manual Multi-Platform Build (Recommended for Development)

**Script:** `scripts/build-multiplatform.sh`

This custom build script provides full control over the build process and creates binaries for all supported platforms:

```bash
cd grafana-aichat-app
./scripts/build-multiplatform.sh
```

**What it does:**
1. Builds frontend (TypeScript → Webpack → dist/)
2. Cross-compiles Go backend for 6 platforms:
   - macOS Intel (darwin/amd64)
   - macOS Apple Silicon (darwin/arm64)
   - Linux AMD64 (linux/amd64)
   - Linux ARM (linux/arm)
   - Linux ARM64 (linux/arm64)
   - Windows AMD64 (windows/amd64)
3. Generates build manifest with file hashes
4. Creates distribution package with all binaries
5. Produces ready-to-distribute zip file

**Advantages:**
- Full transparency and control over build process
- Easy to debug and customize
- No external tool dependencies beyond Go and npm
- Explicit platform targeting
- Clear error messages

**Output:**
```
dist/
├── gpx_grafana-aichat-app_darwin_amd64
├── gpx_grafana-aichat-app_darwin_arm64
├── gpx_grafana-aichat-app_linux_amd64
├── gpx_grafana-aichat-app_linux_arm
├── gpx_grafana-aichat-app_linux_arm64
├── gpx_grafana-aichat-app_windows_amd64.exe
├── go_plugin_build_manifest
├── module.js (and other frontend assets)
└── plugin.json

package/grafana-aichat-app/
└── (all dist files + README.md, CHANGELOG.md, LICENSE)

grafana-aichat-app-1.0.0.zip
```

### Option 2: Grafana Mage Build System (Official Grafana Pattern)

**Script:** Uses `Magefile.go` with Grafana's official build tooling

The plugin includes `Magefile.go` which imports Grafana's standard build system from `grafana-plugin-sdk-go`:

```bash
cd grafana-aichat-app

# Install Mage (if not already installed)
go install github.com/magefile/mage@latest

# Build everything (frontend + all backend platforms)
mage build:all

# Package the plugin
mage package
```

**What it does:**
1. Uses Grafana's official build patterns from plugin SDK
2. Automatically handles frontend and backend builds
3. Creates standardized plugin packages
4. Includes proper signing and manifest generation
5. Follows Grafana plugin registry requirements

**Advantages:**
- Official Grafana-supported approach
- Automatically stays up-to-date with Grafana standards
- Built-in support for plugin signing
- Compatible with Grafana CI/CD pipelines
- Less maintenance overhead

**When to use:**
- Production releases for Grafana plugin registry
- When plugin signing is required
- For organizations using Grafana's standard tooling
- When you want automatic updates to build standards

### Option 3: Package Script (Quick Distribution)

**Script:** `scripts/package.sh`

Updated to include backend compilation and create distribution packages:

```bash
cd grafana-aichat-app
./scripts/package.sh
```

**What it does:**
1. Same as build-multiplatform.sh
2. Specifically focused on creating distributable packages
3. Includes all documentation and metadata

**When to use:**
- Creating distribution packages for manual deployment
- Testing plugin installation process
- Sharing plugin with specific users/teams

## Container Deployment Build

For local development with Podman/Docker, use the integration build script:

```bash
cd grafana-ai
./build-and-install-ai-chat.sh
```

**What it does:**
1. Builds frontend
2. Compiles Go backend for **linux/arm64** only (container platform)
3. Copies to local Grafana plugins directory
4. Provides instructions for container restart

**Backend binary compilation:**
```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o dist/gpx_grafana-aichat-app -ldflags="-w -s" ./pkg
```

**Important:** This only builds the Linux ARM64 binary needed for the local container environment. For full distribution, use Option 1 or 2.

## Build Comparison

| Feature | Manual Build | Mage Build | Package Script | Container Deploy |
|---------|-------------|------------|----------------|------------------|
| **Frontend Build** | ✅ | ✅ | ✅ | ✅ |
| **Backend Build** | ✅ All platforms | ✅ All platforms | ✅ All platforms | ⚠️ Linux ARM64 only |
| **Distribution Package** | ✅ | ✅ | ✅ | ❌ |
| **Plugin Signing** | ❌ Manual | ✅ Built-in | ❌ Manual | ❌ N/A |
| **Customization** | ✅ High | ⚠️ Limited | ✅ High | ✅ High |
| **Maintenance** | ⚠️ Manual updates | ✅ SDK updates | ⚠️ Manual updates | ✅ Low |
| **Use Case** | Development, custom builds | Production, registry | Distribution | Local testing |

## Recommended Workflow

### For Local Development
```bash
# Use container deployment build
cd grafana-ai
./build-and-install-ai-chat.sh
./stop.sh && ./start.sh
```

### For Testing Multi-Platform Support
```bash
# Use manual multi-platform build
cd grafana-aichat-app
./scripts/build-multiplatform.sh
# Test binaries on different platforms
```

### For Production Release
```bash
# Option A: Use Mage (recommended for plugin registry)
cd grafana-aichat-app
mage build:all
mage package

# Option B: Use manual build (more control)
cd grafana-aichat-app
./scripts/build-multiplatform.sh
# Sign the plugin if needed
# Upload to GitHub releases or plugin registry
```

## Backend Binary Details

The Go backend (`gpx_grafana-aichat-app`) provides:

**Features:**
- Chat session management (create, delete, switch)
- Message history persistence (file-based storage)
- REST API endpoints:
  - `GET /health` - Health check
  - `GET /sessions` - List all sessions
  - `POST /sessions` - Create new session
  - `DELETE /sessions/:id` - Delete session
  - `GET /messages/:sessionId` - Get messages
  - `POST /messages` - Add message
  - `PUT /messages/:id` - Update message
  - `DELETE /messages/:id` - Delete message
- Input validation and XSS protection
- Rate limiting and authentication
- Path traversal protection for file storage

**Build flags:**
- `CGO_ENABLED=0` - Static linking, no C dependencies
- `-ldflags="-w -s"` - Strip debug symbols for smaller binaries
- Platform-specific naming: `gpx_grafana-aichat-app_{os}_{arch}`

## Plugin.json Backend Configuration

The `src/plugin.json` declares backend support:

```json
{
  "executable": "gpx_grafana-aichat-app",
  "backend": true
}
```

**Critical:** These fields must be preserved during the webpack build. The build configuration ensures `plugin.json` is copied correctly to `dist/plugin.json`.

## Verifying Backend Integration

After building, verify backend is included:

```bash
# Check dist directory has backend binaries
ls -lh dist/gpx_grafana-aichat-app*

# Check plugin.json declares backend
grep -A2 "executable" dist/plugin.json

# For container deployment, verify binary exists
ls -lh grafana-ai/grafana/plugins/grafana-aichat-app/gpx_grafana-aichat-app
```

## Troubleshooting

### Backend binary not found
- **Symptom:** Grafana logs show "fork/exec gpx_grafana-aichat-app: no such file or directory"
- **Solution:** Run build script that includes Go compilation (not just `npm run build`)

### Wrong platform binary
- **Symptom:** "cannot execute binary file: Exec format error"
- **Solution:** Ensure GOOS/GOARCH match target platform (e.g., linux/arm64 for container)

### Plugin.json missing backend config
- **Symptom:** Backend doesn't start, no API endpoints available
- **Solution:** Verify `dist/plugin.json` contains "executable" and "backend": true

### Build fails with "go: command not found"
- **Solution:** Install Go toolchain: `brew install go` (macOS) or download from golang.org

## Environment Variables

```bash
# Set version for build
export VERSION="1.0.0"
./scripts/build-multiplatform.sh

# Build for specific platform only
GOOS=linux GOARCH=amd64 go build -o dist/gpx_grafana-aichat-app_linux_amd64 ./pkg
```

## Further Reading

- [Grafana Plugin Development](https://grafana.com/docs/grafana/latest/developers/plugins/)
- [Grafana Plugin SDK for Go](https://github.com/grafana/grafana-plugin-sdk-go)
- [Mage Build Tool](https://magefile.org/)
- [Go Cross Compilation](https://go.dev/doc/install/source#environment)
