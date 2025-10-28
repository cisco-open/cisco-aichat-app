# Copyright 2025 Cisco Systems, Inc. and its affiliates
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

#!/bin/bash

set -e

echo "📦 Creating Grafana AI Chat App Distribution Package..."

# Configuration
PLUGIN_NAME="grafana-aichat-app"
VERSION=${VERSION:-"1.0.0"}
BUILD_DIR="dist"
PACKAGE_DIR="package"
BINARY_NAME="gpx_grafana-aichat-app"

# Platform matrix - Multi-platform support for Go backend
declare -a PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm"
    "linux/arm64"
    "windows/amd64"
)

# Clean previous builds
echo "🧹 Cleaning previous builds..."
rm -rf "$BUILD_DIR"
rm -rf "$PACKAGE_DIR"
mkdir -p "$BUILD_DIR"
mkdir -p "$PACKAGE_DIR"

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "📦 Installing dependencies..."
    npm install
fi

# Clear caches to prevent webpack drift
echo "🧹 Clearing webpack and TypeScript caches..."
rm -rf node_modules/.cache/ .tsbuildinfo

# Type check - fail build if errors found
echo "🔍 Type checking..."
if ! npm run typecheck; then
    echo "❌ TypeScript errors found. Fix them before building."
    exit 1
fi

# Build the frontend
echo "🏗️  Building frontend..."
npm run build

# Build Go backends for all platforms
echo "🔧 Building Go backends for all platforms..."

for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "$platform"

    # Determine binary extension and suffix
    BINARY_EXT=""
    BINARY_SUFFIX="_${GOOS}_${GOARCH}"

    if [ "$GOOS" = "windows" ]; then
        BINARY_EXT=".exe"
    fi

    BINARY_OUTPUT="${BINARY_NAME}${BINARY_SUFFIX}${BINARY_EXT}"

    echo "   Building for $GOOS/$GOARCH -> $BINARY_OUTPUT"

    # Build with appropriate CGO settings
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -o "$BUILD_DIR/$BINARY_OUTPUT" \
        -ldflags="-w -s" \
        ./pkg
done

# Generate build manifest
echo "📋 Generating build manifest..."
cat > "$BUILD_DIR/go_plugin_build_manifest" << EOF
# Build manifest for $PLUGIN_NAME $VERSION
# Generated on $(date)
# Platforms: ${PLATFORMS[*]}
EOF

# Add file hashes to manifest
if command -v shasum >/dev/null 2>&1; then
    echo "🔍 Adding file hashes to build manifest..."
    cd "$BUILD_DIR"
    for file in "$BINARY_NAME"*; do
        if [ -f "$file" ]; then
            HASH=$(shasum -a 256 "$file" | cut -d' ' -f1)
            echo "$HASH:$file" >> go_plugin_build_manifest
        fi
    done
    cd ..
fi

# List built binaries
echo "✅ Built binaries:"
ls -lh "$BUILD_DIR/$BINARY_NAME"* 2>/dev/null | awk '{print "   " $9 " (" $5 ")")' || echo "   (Listing binaries...)"

# Copy all files to package directory with proper structure
echo "📦 Creating distribution package..."
# Create plugin directory structure that grafana-cli expects
mkdir -p "$PACKAGE_DIR/$PLUGIN_NAME"
cp -r "$BUILD_DIR/"* "$PACKAGE_DIR/$PLUGIN_NAME/"

# Copy additional distribution files
cp README.md "$PACKAGE_DIR/$PLUGIN_NAME/" 2>/dev/null || echo "⚠️  README.md not found, skipping"
cp CHANGELOG.md "$PACKAGE_DIR/$PLUGIN_NAME/" 2>/dev/null || echo "⚠️  CHANGELOG.md not found, skipping"
cp LICENSE "$PACKAGE_DIR/$PLUGIN_NAME/" 2>/dev/null || echo "⚠️  LICENSE not found, skipping"

# Create installation guide for distribution
cat > "$PACKAGE_DIR/$PLUGIN_NAME/INSTALLATION.md" << 'EOF'
# Grafana AI Chat App Installation

## Multi-Platform Distribution

This package contains pre-built Go backend binaries for multiple platforms:
- **macOS** (Intel & Apple Silicon)
- **Linux** (AMD64, ARM, ARM64)
- **Windows** (AMD64)

Grafana will automatically select the correct binary for your platform.

## Installation

### Method 1: Using grafana-cli (Recommended)

```bash
# Install from local zip file using grafana-cli
grafana-cli --pluginUrl file:///path/to/grafana-aichat-app-1.0.0.zip \
            --pluginsDir "/var/lib/grafana/plugins" \
            plugins install grafana-aichat-app
```

### Method 2: Direct extraction

```bash
# For standard Grafana installation
unzip grafana-aichat-app-1.0.0.zip -d /var/lib/grafana/plugins/

# For Docker/container deployments
unzip grafana-aichat-app-1.0.0.zip -d ./grafana/plugins/
```

### Configuration

1. **Configure unsigned plugin in Grafana:**
   ```ini
   # In grafana.ini
   [plugins]
   allow_loading_unsigned_plugins = grafana-aichat-app
   ```

2. **Restart Grafana**

3. **Configure LLM provider in Apps → LLM App**

4. **Access the AI Chat App via Apps → AI Chat Assistant**

## Requirements

- Grafana 10.4.0+
- LLM App plugin installed and configured
- OpenAI API key or compatible LLM provider

## Features

- Interactive AI chat interface with streaming responses
- Chat history persistence (Go backend with file storage)
- Session management and multiple conversations
- MCP tool integration for enhanced capabilities
- Context-aware conversations
- Message history management with CRUD operations

## Backend Components

This plugin includes a Go backend that provides:
- REST API endpoints for chat session management
- File-based chat history persistence
- Input validation and XSS protection
- Rate limiting and authentication
- Health check endpoints

Backend binary: `gpx_grafana-aichat-app`

For more information, visit: https://github.com/your-org/grafana-aichat-app
EOF

# Create version info
cat > "$PACKAGE_DIR/$PLUGIN_NAME/VERSION" << EOF
Plugin: $PLUGIN_NAME
Version: $VERSION
Build Date: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
Build Host: $(uname -s)/$(uname -m)
Type: App plugin with Go backend
Backend Binary: $BINARY_NAME
Platforms: ${PLATFORMS[*]}
Features: Chat history persistence, session management, REST API
EOF

# Create the distribution zip
echo "🗜️  Creating distribution archive..."
if command -v zip >/dev/null 2>&1; then
    cd "$PACKAGE_DIR"
    zip -r "../${PLUGIN_NAME}-${VERSION}.zip" . >/dev/null
    cd ..

    ZIP_SIZE=$(ls -lh "${PLUGIN_NAME}-${VERSION}.zip" | awk '{print $5}')
    echo "✅ Created ${PLUGIN_NAME}-${VERSION}.zip ($ZIP_SIZE)"
else
    echo "⚠️  zip command not found. Package created in '$PACKAGE_DIR/' directory"
fi

echo ""
echo "🎉 Multi-platform build complete!"
echo "📦 Package: ${PLUGIN_NAME}-${VERSION}.zip"
echo "📏 Size: $(du -h "${PLUGIN_NAME}-${VERSION}.zip" 2>/dev/null | cut -f1 || echo "N/A")"
echo ""
echo "🚀 Distribution ready!"
echo "   ✓ Frontend: React components with streaming chat UI"
echo "   ✓ Backend: Go binaries for ${#PLATFORMS[@]} platforms"
echo "   ✓ Features: Chat persistence, session management, API endpoints"
echo ""
echo "📋 Package contents:"
echo "   - Frontend assets (JavaScript, CSS, images)"
echo "   - Backend binaries for all platforms"
echo "   - README.md, CHANGELOG.md, LICENSE"
echo "   - INSTALLATION.md (setup instructions)"
echo "   - VERSION (build metadata)"
echo "   - go_plugin_build_manifest (binary hashes)"
echo ""
echo "📤 Next steps:"
echo "   1. Test the plugin on different platforms"
echo "   2. Upload to GitHub releases"
echo "   3. Deploy to Grafana instances"