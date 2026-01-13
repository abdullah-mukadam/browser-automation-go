import { useState, useEffect, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import 'reactflow/dist/style.css'
import { Play, Square, Settings, Key, Check, ChevronDown, ChevronUp } from 'lucide-react'
import axios from 'axios'
import WorkflowGraph from '../components/WorkflowGraph'

interface Workflow {
    id: string
    name: string
    start_url: string
    actions: SemanticAction[]
    params: WorkflowParameter[]
}

interface SemanticAction {
    id: string
    sequence_id: number
    action_type: string
    target: {
        tag: string
        selector: string
        text?: string
    }
    value?: string
    interaction_rank: string
}

interface WorkflowParameter {
    name: string
    type: string
    default_value: string
    required: boolean
}

interface WorkflowRun {
    run_id: string
    status: 'pending' | 'running' | 'success' | 'failed' | 'canceled'
    action_results: ActionResult[]
}

interface ActionResult {
    sequence_id: number
    status: 'pending' | 'running' | 'success' | 'failed'
    error_message?: string
}

const API_URL = ''

function ExecutionPage() {
    const { id } = useParams<{ id: string }>()
    const queryClient = useQueryClient()
    const [parameters, setParameters] = useState<Record<string, string>>({})
    const [llmProvider, setLlmProvider] = useState('')
    const [headless, setHeadless] = useState(true)
    const [currentRun, setCurrentRun] = useState<WorkflowRun | null>(null)
    const [runHistory, setRunHistory] = useState<(WorkflowRun & { parameters?: Record<string, string> })[]>([])
    const [ws, setWs] = useState<WebSocket | null>(null)
    const [showApiKeys, setShowApiKeys] = useState(false)
    const [apiKeys, setApiKeys] = useState<Record<string, string>>({ openai: '', anthropic: '', gemini: '' })

    // Fetch workflow
    const { data: workflow, isLoading } = useQuery<Workflow>({
        queryKey: ['workflow', id],
        queryFn: async () => {
            const res = await axios.get(`${API_URL}/api/workflows/${id}`)
            return res.data
        },
        enabled: !!id,
    })

    // Fetch LLM providers
    const { data: providers } = useQuery({
        queryKey: ['llm-providers'],
        queryFn: async () => {
            const res = await axios.get(`${API_URL}/api/llm/providers`)
            return res.data || []
        },
    })

    // Set default provider (excluding ollama)
    useEffect(() => {
        if (providers?.length > 0 && !llmProvider) {
            const valid = providers.find((p: any) => p.name !== 'ollama')
            if (valid) {
                setLlmProvider(valid.name)
            } else {
                // Fallback to first available if all else fails
                const first = providers.find((p: any) => p.available)
                if (first) setLlmProvider(first.name)
            }
        }
    }, [providers, llmProvider])

    // Initialize parameters with defaults
    useEffect(() => {
        if (workflow?.params) {
            const defaults: Record<string, string> = {}
            workflow.params.forEach((p) => {
                defaults[p.name] = p.default_value
            })
            setParameters(defaults)
        }
    }, [workflow])

    // Execute mutation
    const executeMutation = useMutation({
        mutationFn: async () => {
            const res = await axios.post(`${API_URL}/api/workflows/${id}/run`, {
                parameters,
                llm_provider: llmProvider,
                headless,
            })
            return res.data
        },
        onSuccess: (data) => {
            setCurrentRun({
                run_id: data.run_id,
                status: 'running',
                action_results: [],
            })
            // Connect WebSocket for updates
            connectWebSocket(data.run_id)
        },
    })

    // Cancel mutation
    const cancelMutation = useMutation({
        mutationFn: async () => {
            if (currentRun) {
                await axios.post(`${API_URL}/api/runs/${currentRun.run_id}/cancel`)
            }
        },
        onSuccess: () => {
            if (currentRun) {
                setCurrentRun({ ...currentRun, status: 'canceled' })
            }
            ws?.close()
        },
    })

    // WebSocket connection for real-time updates
    const connectWebSocket = useCallback((runId: string) => {
        const wsUrl = `${API_URL.replace('http', 'ws')}/api/runs/${runId}/stream`
        const socket = new WebSocket(wsUrl)

        socket.onmessage = (event) => {
            const data = JSON.parse(event.data)
            if (data.type === 'run_update') {
                const updatedRun: WorkflowRun = {
                    run_id: runId,
                    status: data.payload.status,
                    action_results: data.payload.action_results || [],
                }
                setCurrentRun(updatedRun)

                // When run completes, add to history and reset button state
                if (['success', 'failed', 'canceled'].includes(data.payload.status)) {
                    setRunHistory(prev => [{
                        ...updatedRun,
                        parameters: { ...parameters }
                    }, ...prev])
                    // Reset currentRun to enable Execute button
                    setTimeout(() => setCurrentRun(null), 1000) // Brief delay to show final state
                    socket.close()
                }
            }
        }

        socket.onerror = () => {
            console.error('WebSocket error')
        }

        socket.onclose = () => {
            setWs(null)
        }

        setWs(socket)
    }, [parameters])

    // Cleanup WebSocket on unmount
    useEffect(() => {
        return () => {
            ws?.close()
        }
    }, [ws])

    const handleParameterChange = (name: string, value: string) => {
        setParameters((prev) => ({ ...prev, [name]: value }))
    }

    if (isLoading) {
        return (
            <div className="flex items-center justify-center" style={{ height: 400 }}>
                <div className="spinner" />
            </div>
        )
    }

    if (!workflow) {
        return <div>Workflow not found</div>
    }

    const isRunning = currentRun?.status === 'running'

    return (
        <div>
            <div className="flex justify-between items-center mb-lg">
                <div>
                    <h1 className="page-title">{workflow.name}</h1>
                    <p className="text-muted text-sm">{workflow.start_url}</p>
                </div>
                <div className="flex gap-sm">
                    {isRunning ? (
                        <button
                            className="btn btn-danger"
                            onClick={() => cancelMutation.mutate()}
                            disabled={cancelMutation.isPending}
                        >
                            <Square size={14} />
                            Cancel
                        </button>
                    ) : (
                        <button
                            className="btn btn-primary"
                            onClick={() => executeMutation.mutate()}
                            disabled={executeMutation.isPending}
                        >
                            {executeMutation.isPending ? (
                                <div className="spinner" style={{ width: 14, height: 14 }} />
                            ) : (
                                <Play size={14} />
                            )}
                            Execute
                        </button>
                    )}
                </div>
            </div>

            <div className="grid" style={{ gridTemplateColumns: '300px 1fr', gap: '1.5rem' }}>
                {/* Settings Panel */}
                <div className="card">
                    <h3 className="card-title mb-md">
                        <Settings size={16} /> Settings
                    </h3>

                    {/* LLM Provider */}
                    <div className="form-group">
                        <label className="form-label">LLM Provider</label>
                        <select
                            className="form-input form-select"
                            value={llmProvider}
                            onChange={(e) => setLlmProvider(e.target.value)}
                            disabled={isRunning}
                        >
                            {providers?.filter((p: { name: string }) => p.name !== 'ollama').map((p: { name: string; display: string; available: boolean; has_key: boolean }) => (
                                <option key={p.name} value={p.name} disabled={!p.available}>
                                    {p.display} {!p.has_key ? '(no key)' : !p.available ? '(unavailable)' : ''}
                                </option>
                            ))}
                        </select>
                    </div>

                    {/* API Keys Section */}
                    <div className="form-group mt-md">
                        <button
                            type="button"
                            className="flex items-center gap-sm text-sm"
                            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: 0 }}
                            onClick={() => setShowApiKeys(!showApiKeys)}
                        >
                            <Key size={14} />
                            API Keys
                            {showApiKeys ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                        </button>
                        {showApiKeys && (
                            <div className="mt-md" style={{ padding: '0.75rem', background: 'var(--bg-tertiary)', borderRadius: '0.5rem' }}>
                                {['openai', 'anthropic', 'gemini'].map((provider) => {
                                    const providerInfo = providers?.find((p: { name: string }) => p.name === provider)
                                    const hasKey = providerInfo?.has_key
                                    return (
                                        <div key={provider} className="form-group" style={{ marginBottom: '0.5rem' }}>
                                            <label className="form-label text-sm" style={{ textTransform: 'capitalize' }}>
                                                {provider} API Key
                                                {hasKey && <Check size={12} style={{ color: 'var(--accent-success)', marginLeft: '0.5rem' }} />}
                                            </label>
                                            <div className="flex gap-sm">
                                                <input
                                                    className="form-input"
                                                    type="password"
                                                    value={apiKeys[provider]}
                                                    onChange={(e) => setApiKeys(prev => ({ ...prev, [provider]: e.target.value }))}
                                                    placeholder={hasKey ? '••••••••••' : 'Enter API key'}
                                                    style={{ flex: 1, fontSize: '0.85rem' }}
                                                />
                                                <button
                                                    className="btn btn-sm"
                                                    style={{ padding: '0.25rem 0.5rem', fontSize: '0.8rem' }}
                                                    disabled={!apiKeys[provider]}
                                                    onClick={async () => {
                                                        try {
                                                            await axios.post(`${API_URL}/api/llm/providers/${provider}/key`, { api_key: apiKeys[provider] })
                                                            queryClient.invalidateQueries({ queryKey: ['llm-providers'] })
                                                            setApiKeys(prev => ({ ...prev, [provider]: '' }))
                                                        } catch (err) {
                                                            console.error('Failed to save API key:', err)
                                                        }
                                                    }}
                                                >
                                                    Save
                                                </button>
                                            </div>
                                        </div>
                                    )
                                })}
                            </div>
                        )}
                    </div>

                    {/* Headless Mode */}
                    <div className="form-group">
                        <label className="form-label flex items-center gap-sm">
                            <input
                                type="checkbox"
                                checked={headless}
                                onChange={(e) => setHeadless(e.target.checked)}
                                disabled={isRunning}
                            />
                            Headless Mode
                        </label>
                    </div>

                    {/* Parameters */}
                    {workflow.params && workflow.params.length > 0 && (
                        <>
                            <h4 className="font-medium mt-lg mb-md">Parameters</h4>
                            {workflow.params.map((param) => (
                                <div key={param.name} className="form-group">
                                    <label className="form-label">
                                        {param.name}
                                        {param.required && <span style={{ color: 'var(--accent-error)' }}> *</span>}
                                    </label>
                                    <input
                                        className="form-input"
                                        type={param.type === 'number' ? 'number' : 'text'}
                                        value={parameters[param.name] || ''}
                                        onChange={(e) => handleParameterChange(param.name, e.target.value)}
                                        placeholder={param.default_value}
                                        disabled={isRunning}
                                    />
                                </div>
                            ))}
                        </>
                    )}

                    {/* Run Status */}
                    {currentRun && (
                        <div className="mt-lg">
                            <h4 className="font-medium mb-md">Run Status</h4>
                            <div className={`badge badge-${currentRun.status}`}>
                                {currentRun.status.toUpperCase()}
                            </div>
                        </div>
                    )}
                </div>

                {/* Workflow Graph */}
                <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
                    <WorkflowGraph
                        actions={workflow.actions || []}
                        actionResults={currentRun?.action_results || []}
                    />
                </div>
            </div>

            {/* Run History Table */}
            {runHistory.length > 0 && (
                <div className="card mt-lg">
                    <h3 className="card-title mb-md">Run History</h3>
                    <table className="run-history-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
                        <thead>
                            <tr style={{ borderBottom: '1px solid var(--border-color)' }}>
                                <th style={{ textAlign: 'left', padding: '0.5rem' }}>Run ID</th>
                                <th style={{ textAlign: 'left', padding: '0.5rem' }}>Status</th>
                                <th style={{ textAlign: 'left', padding: '0.5rem' }}>Parameters</th>
                                <th style={{ textAlign: 'left', padding: '0.5rem' }}>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {runHistory.map((run) => (
                                <tr key={run.run_id} style={{ borderBottom: '1px solid var(--border-color)' }}>
                                    <td style={{ padding: '0.5rem', fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                        {run.run_id.slice(0, 8)}...
                                    </td>
                                    <td style={{ padding: '0.5rem' }}>
                                        <span className={`badge badge-${run.status}`}>
                                            {run.status.toUpperCase()}
                                        </span>
                                    </td>
                                    <td style={{ padding: '0.5rem', fontSize: '0.8rem' }}>
                                        {run.parameters && Object.keys(run.parameters).length > 0 ? (
                                            Object.entries(run.parameters).map(([key, value]) => (
                                                <div key={key}>
                                                    <strong>{key}:</strong> {String(value).slice(0, 30)}
                                                    {String(value).length > 30 ? '...' : ''}
                                                </div>
                                            ))
                                        ) : (
                                            <span className="text-muted">None</span>
                                        )}
                                    </td>
                                    <td style={{ padding: '0.5rem' }}>
                                        {run.action_results?.length || 0} / {workflow.actions?.length || 0}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    )
}

export default ExecutionPage
