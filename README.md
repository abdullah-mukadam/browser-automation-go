# Browser Automation Workflow System

A production-ready browser automation system using **Temporal** for workflow orchestration, **Go Rod** for browser control, and multiple **LLM providers** (Ollama, OpenAI, Claude, Gemini) for intelligent code generation.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend UI   │────▶│    API Server   │────▶│   Temporal      │
│   (React)       │     │    (Go)         │     │   Cluster       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                │                        │
                                ▼                        ▼
                        ┌─────────────────┐     ┌─────────────────┐
                        │     MySQL       │     │  Temporal Worker│
                        │   Database      │     │  + Go Rod       │
                        └─────────────────┘     └─────────────────┘
                                                         │
                                                         ▼
                                                ┌─────────────────┐
                                                │     Ollama      │
                                                │   (Local LLM)   │
                                                └─────────────────┘
```

## Features

- **Event Ingestion**: Parse `hybrid_events.json` containing rrweb and custom browser events
- **Semantic Extraction**: Extract meaningful browser actions with robust selector generation
- **LLM Integration**: Generate Go Rod code using Ollama, OpenAI, Claude, or Gemini
- **Temporal Workflows**: Reliable workflow execution with retries and failure handling
- **Real-time UI**: Live workflow visualization with status updates
- **Variable Tokens**: Automatic detection of parameterizable inputs

## Quick Start

### Prerequisites

- Docker and Docker Compose
- (Optional) LLM API keys for OpenAI, Claude, or Gemini

### 1. Clone and Setup

```bash
cd browser-automation-go
cp .env.example .env
# Edit .env to add your LLM API keys (optional)
```

### 2. Start Services

```bash
docker-compose up -d
```

This starts:
- **MySQL** (port 3306) - Database
- **Temporal** (port 7233) - Workflow engine
- **Temporal UI** (port 8233) - Workflow monitoring
- **Ollama** (port 11434) - Local LLM
- **API Server** (port 8080) - REST API
- **Worker** - Temporal worker with Go Rod
- **Frontend** (port 3000) - React UI

### 3. Pull Ollama Model (First Time)

```bash
docker exec automator-ollama ollama pull codellama:13b
docker exec automator-ollama ollama pull nomic-embed-text
```

### 4. Access the UI

Open http://localhost:3000 in your browser.

## Usage

### 1. Upload Recording

Upload your `hybrid_events.json` file through the UI. The system will:
- Parse rrweb and custom events
- Extract semantic actions (clicks, inputs, navigation)
- Identify variable tokens (parameterizable values)
- Generate robust CSS selectors

### 2. Configure Execution

- Select LLM provider for code generation
- Toggle headless mode
- Set parameter values

### 3. Execute Workflow

Click "Execute" to run the workflow. Watch real-time progress in the workflow graph:
- Gray: Pending
- Blue (pulsing): Running
- Green: Success
- Red: Failed

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/workflows` | List workflows |
| POST | `/api/workflows` | Upload events file |
| GET | `/api/workflows/:id` | Get workflow details |
| DELETE | `/api/workflows/:id` | Delete workflow |
| POST | `/api/workflows/:id/run` | Execute workflow |
| GET | `/api/runs` | List workflow runs |
| GET | `/api/runs/:id` | Get run details |
| POST | `/api/runs/:id/cancel` | Cancel running workflow |
| WS | `/api/runs/:id/stream` | Real-time updates |

## LLM Providers

| Provider | Model | Setup |
|----------|-------|-------|
| Ollama | codellama:13b | Local, no API key needed |
| OpenAI | gpt-4-turbo | Set `OPENAI_API_KEY` |
| Claude | claude-3-sonnet | Set `ANTHROPIC_API_KEY` |
| Gemini | gemini-1.5-pro | Set `GEMINI_API_KEY` |

## Project Structure

```
browser-automation-go/
├── cmd/
│   ├── api/main.go          # API server entry point
│   └── worker/main.go       # Temporal worker entry point
├── pkg/
│   ├── models/              # Data structures
│   ├── ingestion/           # Event parsing
│   ├── semantic/            # Semantic extraction + embeddings
│   ├── llm/                 # LLM providers
│   ├── temporal/            # Workflows & activities
│   ├── database/            # MySQL layer
│   └── api/                 # HTTP handlers
├── migrations/              # Database schema
├── frontend/                # React UI
├── docker-compose.yml
├── Dockerfile.api
├── Dockerfile.worker
└── README.md
```

## Development

### Run Locally (without Docker)

1. Start dependencies:
```bash
docker-compose up -d mysql temporal ollama
```

2. Run API server:
```bash
go run ./cmd/api
```

3. Run worker:
```bash
go run ./cmd/worker
```

4. Run frontend:
```bash
cd frontend && npm install && npm run dev
```

### Run Tests

```bash
go test ./pkg/... -v
```

## Troubleshooting

### Ollama Model Not Found

```bash
docker exec automator-ollama ollama pull codellama:13b
```

### Temporal Connection Failed

Wait for Temporal to fully start (check http://localhost:8233)

### Browser Actions Failing

Check worker logs:
```bash
docker logs automator-worker
```

## License

MIT
