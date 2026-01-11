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

// Simple runs listing page
function RunsPage() {
    return (
        <div>
            <h1 className="page-title">Workflow Runs</h1>
            <p className="text-muted">View and manage your workflow executions</p>
        </div>
    )
}

export default App
