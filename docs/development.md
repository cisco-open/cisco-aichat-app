# Development Guide

## Quick Start
1. Clone repository
2. Copy .env.example to .env and configure with your API keys
3. Run ./scripts/dev.sh
4. Access http://localhost:3000

## Development Workflow

### Prerequisites
- Node.js 18+
- Docker or Podman
- Azure OpenAI API key (or compatible LLM provider)

### Setup
```bash
# Install dependencies
npm install

# Copy environment template
cp .env.example .env
# Edit .env with your actual API keys

# Build plugin
npm run build

# Start development environment
./scripts/dev.sh
```

### Development Commands
```bash
# Build plugin
npm run build

# Watch mode for development
npm run dev

# Run tests
npm test

# Type checking
npm run typecheck

# Linting
npm run lint
```

### Plugin Development
- Source code is in `src/`
- Main chat interface: `src/pages/ChatPage.tsx`
- Plugin configuration: `src/plugin.json`
- Build output: `dist/`

### Testing
- Navigate to http://localhost:3000 (admin/admin)
- Go to Apps → AI Chat Assistant
- Test chat functionality with real AI responses

### Troubleshooting
- Ensure LLM App plugin is installed and configured
- Check that AZURE_OPENAI_API_KEY is set correctly
- Verify plugin appears in Administration → Plugins