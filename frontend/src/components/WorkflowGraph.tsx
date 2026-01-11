import { useMemo } from 'react'
import ReactFlow, {
    Node,
    Edge,
    Controls,
    Background,
    BackgroundVariant,
    Position,
    Handle,
    NodeProps,
} from 'reactflow'
import 'reactflow/dist/style.css'
import {
    MousePointer2,
    Type,
    Navigation,
    Keyboard,
    Copy,
    ClipboardPaste,
    CircleDot,
    MousePointerClick,
    Focus,
    Move,
    Play,
    Pause,
    SkipForward,
    FileUp,
    Send,
    ArrowDownToLine,
    Scissors,
    Highlighter
} from 'lucide-react'

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

interface ActionResult {
    sequence_id: number
    status: 'pending' | 'running' | 'success' | 'failed'
    error_message?: string
    retry_count?: number
    screenshot_path?: string
}

interface WorkflowGraphProps {
    actions: SemanticAction[]
    actionResults: ActionResult[]
}

// Custom node component for actions
function ActionNode({ data }: NodeProps) {
    const getIcon = () => {
        switch (data.actionType) {
            case 'click':
                return <MousePointer2 size={14} />
            case 'dblclick':
                return <MousePointerClick size={14} />
            case 'rightclick':
                return <MousePointerClick size={14} />
            case 'input':
                return <Type size={14} />
            case 'navigate':
                return <Navigation size={14} />
            case 'keypress':
                return <Keyboard size={14} />
            case 'copy':
                return <Copy size={14} />
            case 'paste':
                return <ClipboardPaste size={14} />
            case 'cut':
                return <Scissors size={14} />
            case 'focus':
                return <Focus size={14} />
            case 'blur':
                return <CircleDot size={14} />
            case 'drag':
            case 'drop':
                return <Move size={14} />
            case 'select':
                return <Highlighter size={14} />
            case 'media_play':
                return <Play size={14} />
            case 'media_pause':
                return <Pause size={14} />
            case 'media_seek':
                return <SkipForward size={14} />
            case 'file_upload':
                return <FileUp size={14} />
            case 'submit':
                return <Send size={14} />
            case 'scroll':
                return <ArrowDownToLine size={14} />
            default:
                return <CircleDot size={14} />
        }
    }

    const getStatusColor = () => {
        switch (data.status) {
            case 'running':
                return 'var(--accent-info)'
            case 'success':
                return 'var(--accent-success)'
            case 'failed':
                return 'var(--accent-error)'
            default:
                return 'var(--text-muted)'
        }
    }

    const getScreenshotUrl = (path: string) => {
        // Extract filename from path
        const filename = path.split('/').pop()
        return `/api/screenshots/${filename}`
    }

    const getBorderStyle = () => {
        switch (data.status) {
            case 'running':
                return '2px solid var(--accent-info)'
            case 'success':
                return '2px solid var(--accent-success)'
            case 'failed':
                return '2px solid var(--accent-error)'
            default:
                return '1px solid var(--border-color)'
        }
    }

    return (
        <div
            className={`action-node ${data.status}`}
            style={{ border: getBorderStyle() }}
        >
            <Handle type="target" position={Position.Top} style={{ background: '#64748b' }} />

            <div className="flex items-center gap-sm mb-sm">
                <div
                    style={{
                        color: getStatusColor(),
                        display: 'flex',
                        alignItems: 'center',
                    }}
                >
                    {getIcon()}
                </div>
                <span className="action-node-type">{data.actionType}</span>
                <span
                    className="text-sm text-muted"
                    style={{ marginLeft: 'auto' }}
                >
                    #{data.sequence}
                </span>
            </div>

            <div className="action-node-target" title={data.selector}>
                {data.text || data.selector || data.tag}
            </div>

            {data.value && (
                <div className="action-node-value" title={data.value}>
                    "{data.value.length > 30 ? data.value.slice(0, 30) + '...' : data.value}"
                </div>
            )}

            {/* Retry count badge */}
            {data.retryCount > 0 && (
                <div className="action-node-retry">
                    Attempt {data.retryCount + 1}/4
                </div>
            )}

            {/* Error message */}
            {data.status === 'failed' && data.errorMessage && (
                <div
                    className="text-sm mt-sm"
                    style={{ color: 'var(--accent-error)', fontSize: '0.7rem' }}
                >
                    {data.errorMessage.slice(0, 50)}
                </div>
            )}

            {/* Screenshot thumbnail on failure */}
            {data.status === 'failed' && data.screenshotPath && (
                <div className="action-node-screenshot mt-sm">
                    <a
                        href={getScreenshotUrl(data.screenshotPath)}
                        target="_blank"
                        rel="noopener noreferrer"
                        style={{ display: 'block' }}
                    >
                        <img
                            src={getScreenshotUrl(data.screenshotPath)}
                            alt="Failure screenshot"
                            style={{
                                width: '100%',
                                maxWidth: 150,
                                borderRadius: 4,
                                border: '1px solid var(--border-color)',
                            }}
                        />
                    </a>
                </div>
            )}

            <Handle type="source" position={Position.Bottom} style={{ background: '#64748b' }} />
        </div>
    )
}

const nodeTypes = {
    action: ActionNode,
}

function WorkflowGraph({ actions, actionResults }: WorkflowGraphProps) {
    // Create a map of sequence_id to status
    const statusMap = useMemo(() => {
        const map: Record<number, { status: string; errorMessage?: string }> = {}
        actionResults.forEach((result) => {
            map[result.sequence_id] = {
                status: result.status,
                errorMessage: result.error_message,
            }
        })
        return map
    }, [actionResults])

    // Generate nodes
    const nodes: Node[] = useMemo(() => {
        return actions.map((action, index) => {
            const statusInfo = statusMap[action.sequence_id] || { status: 'pending' }

            return {
                id: action.id || `action-${action.sequence_id}`,
                type: 'action',
                position: {
                    x: 250,
                    y: index * 120 + 50,
                },
                data: {
                    actionType: action.action_type,
                    sequence: action.sequence_id,
                    tag: action.target?.tag,
                    selector: action.target?.selector,
                    text: action.target?.text,
                    value: action.value,
                    status: statusInfo.status,
                    errorMessage: statusInfo.errorMessage,
                },
            }
        })
    }, [actions, statusMap])

    // Generate edges
    const edges: Edge[] = useMemo(() => {
        return actions.slice(0, -1).map((action, index) => ({
            id: `edge-${action.sequence_id}`,
            source: action.id || `action-${action.sequence_id}`,
            target: actions[index + 1].id || `action-${actions[index + 1].sequence_id}`,
            animated: statusMap[action.sequence_id]?.status === 'success',
            style: {
                stroke: statusMap[action.sequence_id]?.status === 'success'
                    ? 'var(--accent-success)'
                    : 'var(--border-color)',
                strokeWidth: 2,
            },
        }))
    }, [actions, statusMap])

    return (
        <div className="workflow-graph" style={{ height: '100%', minHeight: 500 }}>
            <ReactFlow
                nodes={nodes}
                edges={edges}
                nodeTypes={nodeTypes}
                fitView
                fitViewOptions={{ padding: 0.3 }}
                defaultViewport={{ x: 0, y: 0, zoom: 0.8 }}
                minZoom={0.3}
                maxZoom={1.5}
            >
                <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="#2a2a3d" />
                <Controls
                    style={{
                        background: 'var(--bg-tertiary)',
                        border: '1px solid var(--border-color)',
                        borderRadius: '0.5rem',
                    }}
                />
            </ReactFlow>
        </div>
    )
}

export default WorkflowGraph
