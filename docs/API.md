# API Reference

Backend REST API for the Grafana AI Chat Assistant plugin.

**Base URL:** `/api/plugins/cisco-aichat-app/resources`

## Endpoints

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "message": "AI Chat Assistant backend is running"
}
```

### GET /settings

Returns provisioned plugin settings.

**Response:**
```json
{
  "provisioned": true,
  "systemPrompt": "You are a helpful assistant.",
  "maxTokens": 4096,
  "temperature": 0.7,
  "enableMcpTools": true
}
```

### GET /sessions

List all chat sessions for the current user.

**Response:**
```json
{
  "sessions": [
    {
      "id": "session-abc123",
      "name": "My Chat",
      "createdAt": "2025-01-15T10:30:00Z",
      "updatedAt": "2025-01-15T11:45:00Z",
      "isActive": true
    }
  ],
  "total": 1
}
```

### POST /sessions

Create a new chat session.

**Request:**
```json
{
  "id": "session-abc123",
  "name": "My Chat"
}
```

**Response:** `201 Created`
```json
{
  "id": "session-abc123",
  "name": "My Chat",
  "messages": []
}
```

### GET /sessions/{id}

Get a specific session with all messages.

**Response:**
```json
{
  "id": "session-abc123",
  "name": "My Chat",
  "messages": [
    {
      "id": "msg-xyz789",
      "role": "user",
      "content": "Hello",
      "timestamp": "2025-01-15T10:30:00Z"
    }
  ]
}
```

### PUT /sessions/{id}

Update a session (name or messages).

**Request:**
```json
{
  "name": "Updated Name",
  "messages": [...]
}
```

**Response:**
```json
{
  "id": "session-abc123",
  "name": "Updated Name",
  "messages": [...]
}
```

### DELETE /sessions/{id}

Delete a session.

**Response:**
```json
{
  "success": true,
  "message": "Session deleted successfully",
  "sessionId": "session-abc123"
}
```

### POST /sessions/{id}/messages

Add a message to a session.

**Request:**
```json
{
  "id": "msg-xyz789",
  "role": "user",
  "content": "Hello, how can you help?"
}
```

**Response:** `201 Created`
```json
{
  "id": "msg-xyz789",
  "role": "user",
  "content": "Hello, how can you help?"
}
```

### PUT /sessions/{id}/messages/{msgId}

Update a message's content.

**Request:**
```json
{
  "content": "Updated message content"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Message updated successfully",
  "messageId": "msg-xyz789"
}
```

### POST /sessions/{id}/activate

Set a session as the active session.

**Response:**
```json
{
  "success": true,
  "message": "Session activated successfully",
  "sessionId": "session-abc123"
}
```

### DELETE /sessions/clear-all

Delete all sessions for the current user.

**Response:**
```json
{
  "success": true,
  "message": "All chat history cleared successfully"
}
```

### POST /telemetry

Report LLM usage telemetry from the frontend.

**Request:**
```json
{
  "model": "gpt-4",
  "requestDurationMs": 1500,
  "inputTokens": 100,
  "outputTokens": 250,
  "ttftMs": 350,
  "success": true
}
```

**Response:**
```json
{
  "success": true
}
```

### GET /metrics

Prometheus metrics endpoint for observability.

**Response:** Prometheus text format
```
# HELP grafana_aichat_plugin_up Plugin health status
# TYPE grafana_aichat_plugin_up gauge
grafana_aichat_plugin_up 1
```

## Error Handling

All endpoints return errors in a standard format:

```json
{
  "error": "error message"
}
```

**HTTP Status Codes:**
- `400` - Bad request (invalid input, malformed JSON)
- `401` - Unauthorized (authentication required)
- `404` - Not found (session or message does not exist)
- `405` - Method not allowed
- `429` - Too many requests (rate limit exceeded)
- `500` - Internal server error

## Validation

- Session IDs: alphanumeric, underscores, hyphens (1-128 chars)
- Message IDs: alphanumeric, underscores, hyphens (1-128 chars)
- Session names: max 256 chars, no control characters
- Message content: max 100,000 chars, no control characters
