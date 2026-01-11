import { useState, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Upload, Trash2, Play, FileJson, Clock, CheckCircle } from 'lucide-react'
import axios from 'axios'

interface Workflow {
    id: string
    name: string
    events_file_path: string
    start_url: string
    is_workflow_generated: boolean
    created_at: string
    actions?: SemanticAction[]
    params?: WorkflowParameter[]
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

const API_URL = ''

function WorkflowsPage() {
    const navigate = useNavigate()
    const queryClient = useQueryClient()
    const [isDragging, setIsDragging] = useState(false)
    const [uploadProgress, setUploadProgress] = useState<string | null>(null)

    // Fetch workflows
    const { data: workflows, isLoading } = useQuery<Workflow[]>({
        queryKey: ['workflows'],
        queryFn: async () => {
            const res = await axios.get(`${API_URL}/api/workflows`)
            return res.data || []
        },
    })

    // Upload mutation
    const uploadMutation = useMutation({
        mutationFn: async (file: File) => {
            const formData = new FormData()
            formData.append('events_file', file)
            formData.append('name', file.name.replace('.json', ''))

            const res = await axios.post(`${API_URL}/api/workflows`, formData, {
                headers: { 'Content-Type': 'multipart/form-data' },
            })
            return res.data
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['workflows'] })
            setUploadProgress(null)
        },
        onError: (error) => {
            console.error('Upload failed:', error)
            setUploadProgress(null)
        },
    })

    // Delete mutation
    const deleteMutation = useMutation({
        mutationFn: async (id: string) => {
            await axios.delete(`${API_URL}/api/workflows/${id}`)
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['workflows'] })
        },
    })

    // Handle file drop
    const handleDrop = useCallback((e: React.DragEvent) => {
        e.preventDefault()
        setIsDragging(false)

        const file = e.dataTransfer.files[0]
        if (file && file.name.endsWith('.json')) {
            setUploadProgress('Uploading...')
            uploadMutation.mutate(file)
        }
    }, [uploadMutation])

    const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0]
        if (file) {
            setUploadProgress('Uploading...')
            uploadMutation.mutate(file)
        }
    }

    const formatDate = (dateStr: string) => {
        return new Date(dateStr).toLocaleDateString('en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        })
    }

    return (
        <div>
            <div className="flex justify-between items-center mb-lg">
                <h1 className="page-title">Workflows</h1>
            </div>

            {/* Upload Area */}
            <div
                className={`upload-area mb-lg ${isDragging ? 'dragging' : ''}`}
                onDragOver={(e) => { e.preventDefault(); setIsDragging(true) }}
                onDragLeave={() => setIsDragging(false)}
                onDrop={handleDrop}
                onClick={() => document.getElementById('file-input')?.click()}
            >
                <input
                    id="file-input"
                    type="file"
                    accept=".json"
                    onChange={handleFileSelect}
                    style={{ display: 'none' }}
                />
                {uploadProgress ? (
                    <div className="flex flex-col items-center gap-md">
                        <div className="spinner" />
                        <p className="text-muted">{uploadProgress}</p>
                    </div>
                ) : (
                    <>
                        <Upload className="upload-icon" size={48} />
                        <p className="upload-text">
                            Drop your <strong>hybrid_events.json</strong> file here or click to browse
                        </p>
                        <p className="upload-hint">
                            Recorded browser sessions will be automatically parsed into semantic actions
                        </p>
                    </>
                )}
            </div>

            {/* Workflows Grid */}
            {isLoading ? (
                <div className="grid grid-3">
                    {[1, 2, 3].map((i) => (
                        <div key={i} className="card">
                            <div className="skeleton" style={{ height: 24, width: '60%', marginBottom: 12 }} />
                            <div className="skeleton" style={{ height: 16, width: '80%', marginBottom: 8 }} />
                            <div className="skeleton" style={{ height: 16, width: '40%' }} />
                        </div>
                    ))}
                </div>
            ) : workflows && workflows.length > 0 ? (
                <div className="grid grid-3">
                    {workflows.map((workflow) => (
                        <div key={workflow.id} className="card">
                            <div className="card-header">
                                <div className="flex items-center gap-sm">
                                    <FileJson size={20} className="text-muted" />
                                    <h3 className="card-title truncate" style={{ maxWidth: 180 }}>
                                        {workflow.name}
                                    </h3>
                                </div>
                                <button
                                    className="btn btn-icon btn-secondary"
                                    onClick={(e) => {
                                        e.stopPropagation()
                                        if (confirm('Delete this workflow?')) {
                                            deleteMutation.mutate(workflow.id)
                                        }
                                    }}
                                >
                                    <Trash2 size={14} />
                                </button>
                            </div>

                            <p className="card-description truncate">
                                {workflow.start_url || 'No URL'}
                            </p>

                            <div className="flex items-center gap-sm mt-md text-sm text-muted">
                                <Clock size={14} />
                                {formatDate(workflow.created_at)}
                            </div>

                            <div className="flex items-center gap-sm mt-md">
                                {workflow.is_workflow_generated ? (
                                    <span className="badge badge-success">
                                        <CheckCircle size={12} />
                                        Generated
                                    </span>
                                ) : (
                                    <span className="badge badge-pending">
                                        Pending
                                    </span>
                                )}
                            </div>

                            <div className="flex gap-sm mt-lg">
                                <button
                                    className="btn btn-primary"
                                    style={{ flex: 1 }}
                                    onClick={() => navigate(`/workflows/${workflow.id}`)}
                                >
                                    <Play size={14} />
                                    Execute
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            ) : (
                <div className="card text-center" style={{ padding: '3rem' }}>
                    <FileJson size={48} className="text-muted" style={{ margin: '0 auto 1rem' }} />
                    <p className="text-muted">No workflows yet. Upload a recording to get started.</p>
                </div>
            )}
        </div>
    )
}

export default WorkflowsPage
