# Contributing to Grafana AI Chat App

Thank you for your interest in contributing to the Grafana AI Chat App! This guide will help you get started with development and contributions.

## Table of Contents

- [Overview](#overview)
- [Development Environment Setup](#development-environment-setup)
- [Project Structure](#project-structure)
- [Development Workflow](#development-workflow)
- [Testing](#testing)
- [Code Standards](#code-standards)
- [Submission Guidelines](#submission-guidelines)
- [Release Process](#release-process)

## Overview

The Grafana AI Chat App is a Grafana application plugin that provides an interactive chat interface powered by Large Language Models (LLMs). It integrates with Grafana's LLM service and can optionally use MCP (Model Context Protocol) tools for enhanced capabilities.

### Key Features

- Interactive AI chat interface with streaming responses
- Integration with Grafana's LLM service (OpenAI, Azure OpenAI, etc.)
- MCP tool calling for enhanced AI capabilities
- Chat session management and persistence
- Configurable system prompts and model selection
- Role-based access control support

## Development Environment Setup

### Prerequisites

- Node.js 18+ and npm
- Grafana 10.0+ development environment
- LLM service configured in Grafana (OpenAI API key or similar)
- Docker or Podman (for testing)

### Initial Setup

1. **Clone the repository**

   ```bash
   git clone <repository-url>
   cd grafana-aichat-app
   ```

2. **Install dependencies**

   ```bash
   npm install
   ```

3. **Set up development environment**

   ```bash
   # Using Docker Compose (recommended)
   docker-compose up -d

   # OR using existing Grafana installation
   # Copy plugin to Grafana plugins directory after building
   ```

4. **Build and install plugin**

   ```bash
   # Development build with watch mode
   npm run dev

   # Production build
   npm run build

   # Build and package for distribution
   ./scripts/build.sh
   ```

### Development Environment Details

For development and testing, you can use:

**Option 1: Docker Compose (Recommended)**
The project includes a `docker-compose.yml` with:

- **Grafana** (port 3000): Development platform with plugin pre-installed
- Pre-configured LLM service settings

**Option 2: Local Grafana Installation**

- Install Grafana locally
- Configure LLM service with API keys
- Copy built plugin to Grafana's plugins directory
- Configure Grafana to allow unsigned plugins

Access Grafana at `http://localhost:3000` (admin/admin) after setup.

### LLM Service Configuration

The AI Chat app requires Grafana's LLM service to be configured:

1. **Navigate to Administration → Plugins → LLM App**
2. **Configure your LLM provider**:
   - OpenAI: Set API key and endpoint
   - Azure OpenAI: Configure deployment settings
   - Other providers: Follow provider-specific setup
3. **Test connection** to ensure LLM service is working

## Project Structure

```
grafana-aichat-app/
├── src/                          # Frontend TypeScript/React code
│   ├── components/               # React components
│   │   ├── ChatSidebar.tsx       # Chat session sidebar
│   │   ├── ChatSettingsModal.tsx # Chat configuration modal
│   │   └── PermissionGuard.tsx   # Access control wrapper
│   ├── pages/                    # Application pages
│   │   └── ChatPage.tsx          # Main chat interface
│   ├── services/                 # Frontend services
│   │   ├── ChatHistoryService.ts # Session management
│   │   ├── ChatSettingsService.ts # Configuration management
│   │   └── MCPIntegrationService.ts # MCP tools integration
│   ├── hooks/                    # React hooks
│   │   └── useChatSession.ts     # Chat state management
│   ├── types/                    # TypeScript type definitions
│   │   └── chat.ts               # Chat message types
│   └── config/                   # Configuration utilities
├── provisioning/                 # Grafana provisioning configuration
│   └── plugins/apps.yaml         # App provisioning example
├── scripts/                      # Build and development scripts
├── tests/                        # Test files and fixtures
└── docs/                         # Documentation
```

### Key Files

- **`src/plugin.json`**: Plugin metadata and configuration
- **`src/pages/ChatPage.tsx`**: Main chat interface with streaming logic
- **`src/services/ChatHistoryService.ts`**: Session persistence and management
- **`src/hooks/useChatSession.ts`**: Chat state management hook
- **`provisioning/plugins/apps.yaml`**: Example app provisioning configuration

## Development Workflow

### Frontend Development (TypeScript/React)

1. **Make changes** to TypeScript/React code in `src/` directory
2. **Build frontend**:

   ```bash
   npm run dev    # Development build with watch mode
   npm run build  # Production build
   ```

3. **Test in browser** at `http://localhost:3000`
4. **Check console** for errors and warnings

### Chat Interface Development

The main chat logic is in `ChatPage.tsx`. Key areas:

- **Streaming Responses**: LLM response handling with `llm.streamChatCompletions`
- **Session Management**: Using `useChatSession` hook for state
- **MCP Integration**: Tool calling via `MCPIntegrationService`
- **Settings Management**: Configuration via `ChatSettingsService`

### Full Development Cycle

1. **Start development environment**:

   ```bash
   docker-compose up -d
   ```

2. **Make your changes** to frontend code

3. **Build and deploy**:

   ```bash
   ./scripts/build.sh           # Build the plugin

   # For Docker Compose
   docker-compose restart grafana

   # For local Grafana
   # Copy dist/ to your Grafana plugins directory
   ```

4. **Test in Grafana**:
   - Navigate to Apps → AI Chat Assistant
   - Test your changes
   - Check browser console and Grafana logs

5. **Iterate** until satisfied

### LLM Integration Testing

Test different LLM scenarios:

1. **Basic Chat**: Simple question-answer interactions
2. **Streaming Responses**: Long responses that stream progressively
3. **MCP Tool Calling**: Questions that trigger tool usage
4. **Error Handling**: Network failures, timeout scenarios
5. **Session Management**: Multiple conversations, persistence

## Testing

### Manual Testing

1. **Basic Functionality**:
   - [ ] Plugin loads without errors
   - [ ] Chat interface displays correctly
   - [ ] Messages send and receive properly
   - [ ] Streaming responses work smoothly

2. **LLM Integration**:
   - [ ] Connects to configured LLM service
   - [ ] Handles streaming responses correctly
   - [ ] Processes tool calls when MCP is available
   - [ ] Shows appropriate error messages

3. **Session Management**:
   - [ ] Creates new chat sessions
   - [ ] Switches between sessions correctly
   - [ ] Persists chat history
   - [ ] Handles session deletion

4. **Configuration**:
   - [ ] Settings modal opens and saves
   - [ ] System prompt configuration works
   - [ ] Model selection functions properly
   - [ ] MCP tools toggle works

### Automated Testing

```bash
# Run frontend tests
npm test

# Run linting
npm run lint

# Build verification
npm run build && echo "Build successful"
```

### Integration Testing

Test with complete environment:

```bash
# Start test environment
docker-compose up -d

# Verify Grafana is running
curl http://localhost:3000/api/health

# Test plugin installation
./scripts/build.sh && docker-compose restart grafana

# Test LLM service
# Verify LLM app is configured in Grafana
```

## Code Standards

### TypeScript/React Guidelines

- **Use TypeScript** for all frontend code with strict typing
- **Follow React hooks patterns** for state management
- **Use Grafana UI components** (`@grafana/ui`) when possible
- **Implement proper error handling** with user-friendly messages
- **Use streaming patterns** for real-time LLM responses
- **Add comprehensive logging** for debugging

### LLM Integration Patterns

- **Check LLM availability** before making requests
- **Handle streaming responses** with proper subscription management
- **Implement timeout handling** for long-running requests
- **Provide user feedback** during processing
- **Handle errors gracefully** with retry options

### Example Code Patterns

#### Chat Service Integration

```typescript
// Good: Proper LLM service integration with error handling
async function sendChatMessage(message: string): Promise<void> {
  try {
    const enabled = await llm.enabled();
    if (!enabled) {
      throw new Error('LLM service is not configured');
    }

    const stream = llm.streamChatCompletions({
      model: llm.Model.BASE,
      messages: [{ role: 'user', content: message }]
    });

    stream.subscribe({
      next: (chunk) => {
        // Handle streaming response
        updateMessage(chunk.content);
      },
      error: (error) => {
        console.error('Streaming error:', error);
        setError('Failed to get response. Please try again.');
      },
      complete: () => {
        setIsLoading(false);
      }
    });
  } catch (error) {
    console.error('Chat error:', error);
    setError('Unable to send message. Please check your configuration.');
  }
}
```

#### Session Management

```typescript
// Good: Proper session state management
const useChatSession = () => {
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [currentSession, setCurrentSession] = useState<ChatSession | null>(null);

  const createNewSession = useCallback(() => {
    const newSession: ChatSession = {
      id: Date.now().toString(),
      name: `Chat ${Date.now()}`,
      messages: [],
      createdAt: Date.now()
    };

    setSessions(prev => [...prev, newSession]);
    setCurrentSession(newSession);

    // Persist to storage
    ChatHistoryService.getInstance().saveSession(newSession);
  }, []);

  return {
    sessions,
    currentSession,
    createNewSession,
    // ... other session methods
  };
};
```

## Submission Guidelines

### Before Submitting

1. **Test thoroughly** with LLM service configured
2. **Verify streaming responses** work correctly
3. **Check for TypeScript compilation errors**
4. **Test session management** and persistence
5. **Verify MCP integration** if applicable
6. **Update documentation** if needed

### Pull Request Process

1. **Create feature branch** from main
2. **Make focused commits** with clear messages
3. **Test changes** with real LLM service
4. **Update CHANGELOG.md** with your changes
5. **Submit pull request** with description of changes

### Pull Request Template

```markdown
## Description
Brief description of changes made.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Performance improvement

## Testing
- [ ] Tested with configured LLM service
- [ ] Verified streaming responses work
- [ ] Tested session management
- [ ] Checked MCP integration (if applicable)
- [ ] No TypeScript compilation errors

## Checklist
- [ ] Code follows project style guidelines
- [ ] Added/updated tests as needed
- [ ] Updated documentation as needed
- [ ] CHANGELOG.md updated
```

## Release Process

### Version Management

The project uses semantic versioning (semver):

- **MAJOR**: Breaking changes to API or configuration
- **MINOR**: New features, backward compatible
- **PATCH**: Bug fixes, backward compatible

### Release Steps

1. **Update version** in `package.json` and `src/plugin.json`
2. **Update CHANGELOG.md** with release notes
3. **Create release build**:

   ```bash
   ./scripts/package.sh  # Creates distribution package
   ```

4. **Test release package** in clean environment
5. **Create git tag** and push to repository
6. **Create GitHub release** with package attachment

### Distribution Package

The release process creates a ZIP package with:

- Frontend build artifacts
- Plugin metadata and documentation
- Installation instructions
- Configuration examples

## Getting Help

### Resources

- **Grafana Plugin Documentation**: <https://grafana.com/docs/grafana/latest/developers/plugins/>
- **Grafana LLM App Documentation**: <https://grafana.com/docs/plugins/grafana-llm-app/>
- **MCP Specification**: <https://spec.modelcontextprotocol.io/>
- **TypeScript/React**: <https://react.dev/> and <https://www.typescriptlang.org/>

### Community

- **Issues**: Report bugs and request features via GitHub issues
- **Discussions**: Use GitHub discussions for questions and ideas
- **Development**: Join development discussions in pull requests

### Troubleshooting

**Plugin not loading**:

- Check Grafana logs for error messages
- Verify plugin is in correct directory structure
- Ensure unsigned plugin configuration is correct

**LLM service issues**:

- Verify LLM app is installed and configured
- Check API keys and endpoint configuration
- Test LLM service connection in Grafana

**Chat not responding**:

- Check browser console for JavaScript errors
- Verify LLM service is enabled and working
- Check network connectivity and API limits

**Build failures**:

- Verify Node.js version meets requirements
- Check for TypeScript compilation errors
- Ensure all dependencies are installed

Thank you for contributing to the Grafana AI Chat App!
