# Browser Automation Workflow System

> **Quick Navigation:**  
> [🚀 Setup](#-setup) • [▶️ Running](#️-running-the-system) • [📖 Usage](#-usage-guide) • [🔌 API](#-api-reference) • [🧪 Testing](#-testing) • [🐛 Troubleshooting](#-troubleshooting)

---

## 📑 Table of Contents

- [Architecture](#️-architecture)
- [Features](#-features)
- [Prerequisites](#-prerequisites)
- [Setup](#-setup)
- [Running the System](#️-running-the-system)
- [Configuration](#-configuration)
- [Usage Guide](#-usage-guide)
- [API Reference](#-api-reference)
- [Project Structure](#-project-structure)
- [Testing](#-testing)
- [Troubleshooting](#-troubleshooting)
- [GPU Support](#-gpu-support)
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
| Docker | 20.10+ | Required |
| Docker Compose | 2.0+ | Required |

---

## 🚀 Setup

### Step 1: Clone & Configure

```bash
git clone https://github.com/abdullah-mukadam/browser-automation-go.git
cd browser-automation-go

# Copy environment template
cp .env.example .env
```

### Step 2: Configure Environment Variables (Optional)

Edit `.env` to set your LLM API keys. **Ollama works locally without any keys.**

```bash
# LLM API Keys (Optional - Ollama works without these)
OPENAI_API_KEY=sk-your-key-here
ANTHROPIC_API_KEY=sk-ant-your-key-here
GEMINI_API_KEY=your-gemini-key-here

# Browser Display Mode
# false = VNC enabled (connect to port 5900 to watch)
# true = headless mode (default)
HEADLESS=true
```

### Step 3: Start All Services

```bash
docker-compose up -d
```

### Step 4: Pull Ollama Models (First Time Only)

```bash
# Pull the code generation model
docker exec automator-ollama ollama pull codellama:13b

# Pull the embedding model
docker exec automator-ollama ollama pull nomic-embed-text
```

> **Note**: Model downloads can take several minutes depending on your internet speed.

---

## ▶️ Running the System

### Start All Services

```bash
docker-compose up -d
```

### Services Overview

| Service | Port | URL | Description |
|---------|------|-----|-------------|
| Frontend | 3000 | http://localhost:3000 | React UI |
| API Server | 8080 | http://localhost:8080 | REST API |
| Temporal UI | 8233 | http://localhost:8233 | Workflow monitoring |
| MySQL | 3306 | - | Database |
| Temporal | 7233 | - | Workflow engine |
| Ollama | 11434 | - | Local LLM |
| Worker (VNC) | 5900 | vnc://localhost:5900 | Browser automation |

### Verify Services

```bash
# Check all containers are running
docker-compose ps

# View logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f api
docker-compose logs -f worker
```

### Stop Services

```bash
docker-compose down
```

### Reset Everything (Including Database)

```bash
docker-compose down -v
docker-compose up -d
```

---

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | API server port |
| `MYSQL_DSN` | See `.env.example` | MySQL connection string |
| `TEMPORAL_HOST` | `temporal:7233` | Temporal server address |
| `OLLAMA_HOST` | `http://ollama:11434` | Ollama server URL |
| `OLLAMA_MODEL` | `codellama:13b` | Ollama model for code generation |
| `OPENAI_API_KEY` | - | OpenAI API key (optional) |
| `ANTHROPIC_API_KEY` | - | Anthropic API key (optional) |
| `GEMINI_API_KEY` | - | Google Gemini API key (optional) |
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

### Step 1: Access the UI

Open **http://localhost:3000** in your browser. You'll see the **Workflows Dashboard**.

### Step 2: Create a New Workflow

1. Click the **"New Workflow"** or **"Upload"** button
2. Select your recording file:
   - `.json` — Human-readable JSON format
   - `.bin` — Protobuf binary format (smaller, faster)
3. Enter a **name** for your workflow
4. Click **Upload**

The system automatically parses your recording and extracts semantic actions.

### Step 3: Review Extracted Actions

After upload, you'll see the **Workflow Detail** page showing:

| Column | Description |
|--------|-------------|
| **#** | Action sequence number |
| **Type** | Action type (click, input, navigation, etc.) |
| **Target** | CSS selector or element description |
| **Value** | Input value or URL (if applicable) |

**Extracted action types:**
- 🖱️ **Click** — Button clicks, link clicks
- ⌨️ **Input** — Text typed into fields
- 🔗 **Navigation** — Page URL changes
- 📋 **Copy/Paste** — Clipboard operations
- 🔀 **Tab Switch** — Browser tab changes

### Step 4: Configure Execution Settings

Before running, configure:

| Setting | Options | Description |
|---------|---------|-------------|
| **LLM Provider** | Ollama, OpenAI, Claude, Gemini | Which AI generates the Go Rod code |
| **Headless** | On/Off | Hide or show browser during execution |
| **Parameters** | Editable values | Modify dynamic inputs (search terms, dates) |

> **Tip**: Parameters are auto-detected from your recording. Edit them to run the same workflow with different values!

### Step 5: Execute the Workflow

1. Click the **"Execute"** or **"Run"** button
2. You'll be redirected to the **Execution Page**
3. Watch the **workflow graph** update in real-time:

| Status | Visual | Meaning |
|--------|--------|---------|
| Pending | ⬜ Gray | Waiting to execute |
| Running | 🔵 Blue (pulsing) | Currently executing |
| Success | ✅ Green | Completed successfully |
| Failed | 🔴 Red | Error occurred |

4. Click on any action node to see:
   - Generated Go Rod code
   - Screenshot (if captured)
   - Error message (if failed)

### Step 6: View Run History

Navigate to **Runs** to see all workflow executions:
- Filter by workflow or status
- View detailed logs for each run
- Re-run previous workflows

### Step 7: Watch Live via VNC (Optional)

To see the browser automation in real-time:

1. Set `HEADLESS=false` in your `.env` file
2. Restart the worker:
   ```bash
   docker-compose up -d worker
   ```
3. Connect via VNC:
   ```bash
   # macOS
   open vnc://localhost:5900
   
   # Linux
   vncviewer localhost:5900
   
   # Windows - use any VNC client
   ```
4. Watch the browser perform your recorded actions!

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
├── docker-compose.yml           # Container orchestration
├── Dockerfile.api               # API server container
├── Dockerfile.worker            # Worker container (with Chromium)
└── .env.example                 # Environment template
```

---

## 🧪 Testing

### Run Go Tests in Docker

```bash
# Build and run tests
docker-compose run --rm api go test ./pkg/... -v
```

### Test API Endpoints

```bash
# List workflows
curl http://localhost:8080/api/workflows

# Upload a recording
curl -X POST http://localhost:8080/api/workflows \
  -F "file=@hybrid_events.json"
```

### View Worker Logs

```bash
docker-compose logs -f worker
```

### View All Logs

```bash
docker-compose logs -f
```

---

## 🐛 Troubleshooting

### Ollama Model Not Found

```bash
docker exec automator-ollama ollama pull codellama:13b
```

### Temporal Connection Failed

```bash
# Check Temporal logs
docker-compose logs temporal

# Wait for startup and access UI
open http://localhost:8233
```

### Browser Actions Failing

```bash
docker-compose logs -f worker
```

### Database Connection Issues

```bash
# Check MySQL status
docker-compose ps mysql
docker-compose logs mysql

# Reset database
docker-compose down -v
docker-compose up -d
```

### Frontend Not Loading

```bash
docker-compose logs frontend
docker-compose build frontend
docker-compose up -d frontend
```

### VNC Not Connecting

```bash
# Ensure HEADLESS=false in .env, then restart
docker-compose up -d worker
```

### Rebuild All Services

```bash
docker-compose build
docker-compose up -d
```

---

## 🎮 GPU Support

Enable GPU acceleration for faster LLM inference. Edit `docker-compose.yml`:

```yaml
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
