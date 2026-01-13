import React from 'react'
import { Routes, Route, NavLink } from 'react-router-dom'
import { Workflow, Play } from 'lucide-react'
import WorkflowsPage from './pages/WorkflowsPage'
import ExecutionPage from './pages/ExecutionPage'

function App() {
    return (
        <div className="app">
            <header className="header">
                <div className="container header-content">
                    <NavLink to="/" className="logo">
                        <div className="logo-icon">
                            <Workflow size={18} color="white" />
                        </div>
                        <span>Browser Automator</span>
                    </NavLink>

                    <nav className="nav">
                        <NavLink
                            to="/"
                            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
                        >
                            <Workflow size={16} />
                            Workflows
                        </NavLink>
                        <NavLink
                            to="/runs"
                            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
                        >
                            <Play size={16} />
                            Runs
                        </NavLink>
                    </nav>
                </div>
            </header>

            <main className="main">
                <div className="container">
                    <Routes>
                        <Route path="/" element={<WorkflowsPage />} />
                        <Route path="/workflows/:id" element={<ExecutionPage />} />
                        <Route path="/runs" element={<RunsPage />} />
                        <Route path="/runs/:id" element={<ExecutionPage />} />
                    </Routes>
                </div>
            </main>
        </div>
    )
}

// Runs listing page
function RunsPage() {
    const [runs, setRuns] = React.useState<any[]>([])
    const [loading, setLoading] = React.useState(true)

    React.useEffect(() => {
        fetch('/api/runs')
            .then(res => res.json())
            .then(data => {
                setRuns(data || [])
                setLoading(false)
            })
            .catch(() => setLoading(false))
    }, [])

    const getStatusBadge = (status: string) => {
        const colors: Record<string, string> = {
            pending: '#f59e0b',
            running: '#3b82f6',
            success: '#10b981',
            failed: '#ef4444',
            canceled: '#6b7280'
        }
        return (
            <span style={{
                padding: '0.25rem 0.5rem',
                borderRadius: '0.25rem',
                fontSize: '0.75rem',
                fontWeight: 500,
                backgroundColor: `${colors[status]}20`,
                color: colors[status]
            }}>
                {status?.toUpperCase()}
            </span>
        )
    }

    if (loading) return <div>Loading...</div>

    return (
        <div>
            <h1 className="page-title">Workflow Runs</h1>
            <p className="text-muted mb-lg">View and manage your workflow executions</p>

            {runs.length === 0 ? (
                <div className="card" style={{ textAlign: 'center', padding: '2rem' }}>
                    <p className="text-muted">No runs found. Execute a workflow to see runs here.</p>
                </div>
            ) : (
                <div className="card" style={{ padding: 0 }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                        <thead>
                            <tr style={{ borderBottom: '1px solid var(--border-color)' }}>
                                <th style={{ padding: '1rem', textAlign: 'left' }}>Run ID</th>
                                <th style={{ padding: '1rem', textAlign: 'left' }}>Workflow</th>
                                <th style={{ padding: '1rem', textAlign: 'left' }}>Status</th>
                                <th style={{ padding: '1rem', textAlign: 'left' }}>Started</th>
                            </tr>
                        </thead>
                        <tbody>
                            {runs.map((run: any) => (
                                <tr key={run.id} style={{ borderBottom: '1px solid var(--border-color)' }}>
                                    <td style={{ padding: '1rem' }}>
                                        <a href={`/workflows/${run.workflow_id}`} style={{ color: 'var(--accent-primary)' }}>
                                            {run.id?.substring(0, 8)}...
                                        </a>
                                    </td>
                                    <td style={{ padding: '1rem' }}>{run.workflow_id?.substring(0, 8)}...</td>
                                    <td style={{ padding: '1rem' }}>{getStatusBadge(run.status)}</td>
                                    <td style={{ padding: '1rem' }}>
                                        {run.started_at ? new Date(run.started_at).toLocaleString() : '-'}
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

export default App
