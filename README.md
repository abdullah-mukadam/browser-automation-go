# Browser Automation Workflow System

> **Quick Nav:** [🚀 Setup](#-quick-setup) • [📖 Usage](#-usage-guide) • [🏗️ Architecture](#️-architecture) • [🔌 API](#-api-reference)

A production-ready system that converts recorded browser sessions into executable, repeatable Go Rod workflows. Orchestrated by **Temporal** and powered by **LLMs** (Ollama, OpenAI, Claude, Gemini).

---

## ✨ Features

- **Smart Ingestion**: Instant upload of large recordings (JSON/Bin) with client-side optimization.
- **Semantic Extraction**: Converts raw events into robust, reliable browser actions.
- **AI Code Generation**: Generates Go Rod code using your preferred LLM.
- **Reliable Execution**: Temporal-backed workflows with retries, cancellation, and error handling.
- **Dynamic Parameters**: Inject runtime variables (e.g., search terms, credentials) to override recorded values.
- **Live Observability**: Real-time execution graph, logs, and optional VNC playback.

## 🚀 Quick Setup

1. **Clone & Start**
   ```bash
   git clone https://github.com/abdullah-mukadam/browser-automation-go.git
   cd browser-automation-go
   cp .env.example .env
   docker-compose up -d
   ```

2. **Access UI**
   - Open **http://localhost:3000**
   - (Optional) Configure API keys for OpenAI/Claude/Gemini directly in the UI settings.
   - For local inference, `automator-ollama` runs automatically.

3. **Verify Services**
   - **Frontend**: http://localhost:3000
   - **API**: http://localhost:8080
   - **Temporal UI**: http://localhost:8233

## 📖 Usage Guide

### 1. Create Workflow
Upload a recording file (`.json` or `.bin`). The system filters noise (like high-frequency mouse moves) and extracts key actions.

### 2. Configure
- **LLM Provider**: Select Ollama (local) or a cloud provider.
- **Parameters**: The system detects variable inputs. You can override these values before running.

### 3. Execute
- Run the workflow.
- Watch the real-time graph update as actions complete.
- **Cancel** anytime if needed.

### 4. Watch Live (Optional)
To view the browser:
1. Set `HEADLESS=false` in `.env`.
2. Restart worker: `docker-compose up -d worker`.
3. Connect via VNC: `vnc://localhost:5900`.

## 🏗️ Architecture

```
[Frontend (React)] -> [API (Go)] -> [Temporal Cluster]
                                         |
                                         v
                                  [Worker (Go Rod)]
                                         |
                                         +-> [LLM Provider]
                                         +-> [Browser (Chrome)]
```

## 🔌 API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/workflows` | Upload recording |
| `POST` | `/api/workflows/{id}/run` | Execute workflow |
| `POST` | `/api/runs/{id}/cancel` | Cancel execution |
| `GET` | `/api/llm/providers` | List/Config LLMs |

## 🧪 Testing & Logs

```bash
# View logs
docker-compose logs -f worker

# Run tests
docker-compose run --rm api go test ./pkg/...
```

## 📝 Notes

<!-- Write your notes here -->

---

## License

MIT
