# Browser Automation Workflow System

> **Quick Navigation:**  
> [🚀 Quick Start](#-quick-start-docker) • [💻 Local Development](#-local-development-setup) • [🔧 Configuration](#-configuration-reference) • [📖 Usage](#-usage-guide) • [🔌 API](#-api-reference) • [🐛 Troubleshooting](#-troubleshooting)

---

## 📑 Table of Contents

- [Architecture](#️-architecture)
- [Features](#-features)
- [Prerequisites](#-prerequisites)
- [Quick Start (Docker)](#-quick-start-docker)
- [Local Development Setup](#-local-development-setup)
- [Configuration Reference](#-configuration-reference)
- [Usage Guide](#-usage-guide)
- [API Reference](#-api-reference)
- [Project Structure](#-project-structure)
- [Testing](#-testing)
- [Troubleshooting](#-troubleshooting)
- [Building for Production](#-building-for-production)
- [GPU Support for Ollama](#-gpu-support-for-ollama)
- [License](#-license)

---

A production-ready browser automation system that converts recorded browser sessions into executable, repeatable workflows. Built with **Go**, **Temporal** for workflow orchestration, **Go Rod** for browser control, and multiple **LLM providers** for intelligent code generation.

## 🏗️ Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend UI   │────▶│    API Server   │────▶│   Temporal      │
│   (React/Vite)  │     │    (Go)         │     │   Cluster       │
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
                                                │   LLM Provider  │
                                                │ (Ollama/OpenAI/ │
                                                │  Claude/Gemini) │
                                                └─────────────────┘
```

## ✨ Features

- **Event Ingestion**: Parse `hybrid_events.json` or `.bin` (protobuf) files containing rrweb and custom browser events
- **Semantic Extraction**: Extract meaningful browser actions with robust selector generation
- **LLM Integration**: Generate Go Rod automation code using Ollama, OpenAI, Claude, or Gemini
- **Temporal Workflows**: Reliable workflow execution with retries and failure handling
- **Real-time UI**: Live workflow visualization with status updates
- **Variable Tokens**: Automatic detection of parameterizable inputs
- **VNC Support**: Watch browser automation in real-time via VNC viewer

---

## 📋 Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Docker | 20.10+ | Required for containerized deployment |
| Docker Compose | 2.0+ | Required for orchestrating services |
| Go | 1.23+ | Only for local development |
| Node.js | 20+ | Only for frontend development |
| npm | 10+ | Only for frontend development |

---

## 🚀 Quick Start (Docker)

### Step 1: Clone & Configure

```bash
git clone <repository-url>
cd browser-automation-go

# Copy environment template
cp .env.example .env
```

### Step 2: Configure Environment Variables

Edit `.env` and set your LLM API keys (optional—Ollama works locally without keys):

```bash
# API Server
PORT=8080

# MySQL Database
MYSQL_DSN=automator:automator@tcp(localhost:3306)/automator?parseTime=true

# Temporal
TEMPORAL_HOST=localhost:7233

# Ollama (Local LLM - no API key needed)
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=codellama:13b

# LLM API Keys (Optional)
OPENAI_API_KEY=sk-your-key-here
ANTHROPIC_API_KEY=sk-ant-your-key-here
GEMINI_API_KEY=your-gemini-key-here

# Browser Display Mode
# false = VNC enabled (connect to port 5900)
# true = headless mode (default)
HEADLESS=true
```

### Step 3: Start All Services

```bash
docker-compose up -d
```

This starts the following services:

| Service | Port | Description |
|---------|------|-------------|
| MySQL | 3306 | Database storage |
| Temporal | 7233 | Workflow engine |
| Temporal UI | 8233 | Workflow monitoring dashboard |
| Ollama | 11434 | Local LLM server |
| API Server | 8080 | REST API backend |
| Worker | 5900 (VNC) | Temporal worker + Go Rod browser |
| Frontend | 3000 | React UI |

### Step 4: Pull Ollama Models (First Time Only)

```bash
# Pull the code generation model
docker exec automator-ollama ollama pull codellama:13b

# Pull the embedding model (for semantic search)
docker exec automator-ollama ollama pull nomic-embed-text
```

> **Note**: Model downloads can take several minutes depending on your internet speed.

### Step 5: Verify Services

```bash
# Check all containers are running
docker-compose ps

# Check API health
curl http://localhost:8080/api/workflows

# View Temporal UI
open http://localhost:8233
```

### Step 6: Access the Application

Open **http://localhost:3000** in your browser.

---

## 💻 Local Development Setup

For development without Docker containers for the API/Worker/Frontend:

### Step 1: Start Infrastructure Services

```bash
docker-compose up -d mysql temporal temporal-ui ollama
```

### Step 2: Wait for Services to Initialize

```bash
# Wait for MySQL to be ready
docker-compose logs -f mysql | grep -m1 "ready for connections"

# Wait for Temporal to be ready
docker-compose logs -f temporal | grep -m1 "started"
```

### Step 3: Run API Server

```bash
# From project root
go run ./cmd/api
```

### Step 4: Run Temporal Worker

```bash
# In a new terminal, from project root
go run ./cmd/worker
```

### Step 5: Run Frontend

```bash
cd frontend
npm install
npm run dev
```

The frontend will be available at **http://localhost:5173** (Vite dev server).

---

## 🔧 Configuration Reference

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | API server port |
| `MYSQL_DSN` | See `.env.example` | MySQL connection string |
| `TEMPORAL_HOST` | `localhost:7233` | Temporal server address |
| `OLLAMA_HOST` | `http://localhost:11434` | Ollama server URL |
| `OLLAMA_MODEL` | `codellama:13b` | Ollama model for code generation |
| `OPENAI_API_KEY` | - | OpenAI API key (optional) |
| `OPENAI_MODEL` | `gpt-4-turbo-preview` | OpenAI model name |
| `ANTHROPIC_API_KEY` | - | Anthropic API key (optional) |
| `ANTHROPIC_MODEL` | `claude-3-sonnet-20240229` | Claude model name |
| `GEMINI_API_KEY` | - | Google Gemini API key (optional) |
| `GEMINI_MODEL` | `gemini-1.5-pro` | Gemini model name |
| `SCREENSHOT_DIR` | `/tmp/screenshots` | Directory for screenshots |
| `HEADLESS` | `true` | Run browser in headless mode |

### LLM Providers

| Provider | Model | Setup | Best For |
|----------|-------|-------|----------|
| Ollama | `codellama:13b` | Local, no API key | Free, privacy-focused |
| OpenAI | `gpt-4-turbo` | Set `OPENAI_API_KEY` | Highest quality |
| Claude | `claude-3-sonnet` | Set `ANTHROPIC_API_KEY` | Long context tasks |
| Gemini | `gemini-1.5-pro` | Set `GEMINI_API_KEY` | Cost-effective |

---

## 📖 Usage Guide

### 1. Record Browser Session

Use the companion **Semantic Recorder** Chrome extension to capture browser interactions. The extension generates `hybrid_events.json` or `.bin` files containing:
- **rrweb events**: DOM snapshots and mutations
- **Custom events**: Clicks, inputs, copy/paste, navigation

### 2. Upload Recording

Upload your recording file through the UI:
- Supported formats: `.json` and `.bin` (protobuf)
- The system automatically parses and extracts semantic actions

### 3. Review Extracted Actions

The system extracts:
- **Click actions**: Button clicks, link navigation
- **Input actions**: Text input, form filling
- **Navigation**: URL changes, tab switches
- **Clipboard**: Copy/paste operations

### 4. Configure Execution

- **LLM Provider**: Select which AI generates the automation code
- **Headless Mode**: Toggle visibility of browser during execution
- **Parameters**: Edit dynamic values (dates, search terms, etc.)

### 5. Execute Workflow

Click "Execute" and monitor real-time progress:
- ⬜ **Gray**: Pending
- 🔵 **Blue (pulsing)**: Running
- ✅ **Green**: Success
- 🔴 **Red**: Failed

### 6. Watch Live (Optional)

With `HEADLESS=false`, connect via VNC to watch automation:
```bash
# macOS
open vnc://localhost:5900

# Linux
vncviewer localhost:5900

# Windows
# Use any VNC client to connect to localhost:5900
```

---

## 🔌 API Reference

### Workflows

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/workflows` | List all workflows |
| `POST` | `/api/workflows` | Upload events file |
| `GET` | `/api/workflows/:id` | Get workflow details |
| `DELETE` | `/api/workflows/:id` | Delete workflow |
| `POST` | `/api/workflows/:id/run` | Execute workflow |

### Runs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/runs` | List workflow runs |
| `GET` | `/api/runs/:id` | Get run details |
| `POST` | `/api/runs/:id/cancel` | Cancel running workflow |
| `WS` | `/api/runs/:id/stream` | Real-time status updates |

---

## 📁 Project Structure

```
browser-automation-go/
├── cmd/
│   ├── api/main.go              # API server entry point
│   ├── worker/main.go           # Temporal worker entry point
│   └── test_parser/main.go      # Parser testing utility
├── pkg/
│   ├── api/                     # HTTP handlers & routing
│   ├── database/                # MySQL database layer
│   ├── ingestion/               # Event file parsing (JSON + Protobuf)
│   ├── llm/                     # LLM provider integrations
│   ├── models/                  # Data structures
│   ├── proto/                   # Generated protobuf code
│   ├── semantic/                # Action extraction & embeddings
│   └── temporal/                # Workflows & activities
├── migrations/                  # MySQL schema migrations
├── frontend/                    # React + Vite + TypeScript UI
│   ├── src/
│   │   ├── pages/               # Page components
│   │   └── ...
│   ├── package.json
│   └── vite.config.ts
├── schema.proto                 # Protobuf schema for events
├── docker-compose.yml           # Container orchestration
├── Dockerfile.api               # API server container
├── Dockerfile.worker            # Worker container (with Chromium)
├── .env.example                 # Environment template
└── README.md
```

---

## 🧪 Testing

### Run Go Tests

```bash
go test ./pkg/... -v
```

### Test Event Parser

```bash
go run ./cmd/test_parser hybrid_events.json
```

---

## 🐛 Troubleshooting

### Ollama Model Not Found

```bash
docker exec automator-ollama ollama pull codellama:13b
```

### Temporal Connection Failed

Wait for Temporal to fully initialize:
```bash
docker-compose logs -f temporal
# Look for "started" message
```

Access Temporal UI at http://localhost:8233 to verify.

### Browser Actions Failing

Check worker logs:
```bash
docker logs automator-worker -f
```

### Database Connection Issues

```bash
# Verify MySQL is running
docker-compose ps mysql

# Check MySQL logs
docker-compose logs mysql

# Reset database (destructive)
docker-compose down -v
docker-compose up -d
```

### Frontend Not Loading

```bash
# Check frontend container logs
docker logs automator-frontend

# Rebuild frontend
docker-compose build frontend
docker-compose up -d frontend
```

### VNC Not Connecting

Ensure `HEADLESS=false` in your `.env`:
```bash
# Restart worker with VNC enabled
HEADLESS=false docker-compose up -d worker
```

---

## 🔨 Building for Production

### Build Docker Images

```bash
# Build all images
docker-compose build

# Build specific service
docker-compose build api
docker-compose build worker
docker-compose build frontend
```

### Build Go Binaries

```bash
# API Server
CGO_ENABLED=0 GOOS=linux go build -o bin/api ./cmd/api

# Worker
CGO_ENABLED=0 GOOS=linux go build -o bin/worker ./cmd/worker
```

### Build Frontend

```bash
cd frontend
npm run build
# Output in frontend/dist/
```

---

## 📊 GPU Support for Ollama

To enable GPU acceleration for faster LLM inference:

```yaml
# In docker-compose.yml, uncomment:
ollama:
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu]
```

Then restart:
```bash
docker-compose up -d ollama
```

---

## 📄 License

MIT
