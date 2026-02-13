// GRID Dashboard — Vue 3 SPA (CDN, no build step)

const { createApp, ref, computed, watch, onMounted, onUnmounted, nextTick } = Vue;

// ─── API Client ────────────────────────────────────────────

const api = {
    async get(path) {
        const res = await fetch(path);
        if (!res.ok) {
            const body = await res.json().catch(() => ({}));
            throw new Error(body.error || `HTTP ${res.status}`);
        }
        return res.json();
    },
    async post(path, body) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const data = await res.json().catch(() => ({}));
            throw new Error(data.error || `HTTP ${res.status}`);
        }
        return res.json();
    },
};

// ─── Helpers ───────────────────────────────────────────────

function timeAgo(ts) {
    if (!ts) return '—';
    const now = Date.now();
    const then = new Date(ts).getTime();
    const diff = Math.max(0, now - then);
    const secs = Math.floor(diff / 1000);
    if (secs < 60) return `${secs}s ago`;
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    const days = Math.floor(hrs / 24);
    return `${days}d ago`;
}

function shortId(id) {
    if (!id) return '—';
    return id.substring(0, 8);
}

function statusColor(status) {
    const map = {
        success: 'green', completed: 'green', online: 'green',
        failed: 'red', offline: 'red',
        running: 'blue',
        pending: 'amber',
        cancelled: 'gray', skipped: 'gray',
    };
    return map[status] || 'gray';
}

function statusIcon(status) {
    const map = {
        success: '\u2713', completed: '\u2713',
        failed: '\u2717',
        running: '\u25CB',
        pending: '\u2022',
        cancelled: '\u2014', skipped: '\u2014',
    };
    return map[status] || '?';
}

function flatLeafCount(phases) {
    if (!phases) return 0;
    let count = 0;
    for (const p of phases) {
        if (p.backend) {
            count++;
        } else if (p.tasks && p.tasks.length > 0) {
            count += flatLeafCount(p.tasks);
        }
    }
    return count;
}

function phaseLabel(phase) {
    if (phase.backend) {
        return `${phase.backend}.${phase.action}`;
    }
    const n = flatLeafCount(phase.tasks);
    return `Pipeline (${n} step${n !== 1 ? 's' : ''})`;
}

function formatParams(params) {
    if (!params || Object.keys(params).length === 0) return '';
    return Object.entries(params).map(([k, v]) => `${k}=${v}`).join(', ');
}

function uptime(ts) {
    if (!ts) return '—';
    const now = Date.now();
    const then = new Date(ts).getTime();
    const diff = Math.max(0, now - then);
    const secs = Math.floor(diff / 1000);
    const days = Math.floor(secs / 86400);
    const hrs = Math.floor((secs % 86400) / 3600);
    const mins = Math.floor((secs % 3600) / 60);
    if (days > 0) return `${days}d ${hrs}h`;
    if (hrs > 0) return `${hrs}h ${mins}m`;
    return `${mins}m`;
}

function formatBytes(bytes) {
    if (bytes == null) return '—';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
    return (bytes / 1073741824).toFixed(2) + ' GB';
}

function formatNumber(n) {
    if (n == null) return '—';
    return n.toLocaleString();
}

function formatTime(ts) {
    if (!ts) return '—';
    return new Date(ts).toLocaleString();
}

function jobProgress(job) {
    if (!job || !job.tasks) return { done: 0, total: 0 };
    const total = flatLeafCount(job.tasks);
    const done = job.results ? Object.keys(job.results).length : 0;
    return { done, total };
}

// ─── App ───────────────────────────────────────────────────

const app = createApp({
    template: `
    <div class="header">
        <div class="header-left">
            <div class="logo">GRID</div>
            <div class="nav">
                <button class="nav-btn" :class="{ active: view === 'dashboard' }"
                    @click="view = 'dashboard'">Dashboard</button>
                <button class="nav-btn" :class="{ active: view === 'jobs' }"
                    @click="view = 'jobs'">Jobs</button>
                <button class="nav-btn" :class="{ active: view === 'cluster' }"
                    @click="view = 'cluster'">Cluster</button>
            </div>
        </div>
        <div class="header-right">
            <button class="btn btn-sm" @click="openSubmitForm">New Job</button>
        </div>
    </div>

    <div v-if="error" class="error-toast">{{ error }}</div>

    <div class="main">
        <!-- Dashboard View -->
        <div v-if="view === 'dashboard'">
            <div class="summary-row">
                <div class="summary-card">
                    <div class="label">Controllers</div>
                    <div class="value blue">{{ controllers.length }}</div>
                </div>
                <div class="summary-card">
                    <div class="label">Nodes Online</div>
                    <div class="value green">{{ onlineNodes }}</div>
                </div>
                <div class="summary-card">
                    <div class="label">Active Jobs</div>
                    <div class="value blue">{{ activeJobs }}</div>
                </div>
                <div class="summary-card">
                    <div class="label">Failed Jobs</div>
                    <div class="value red">{{ failedJobs }}</div>
                </div>
            </div>

            <div class="dashboard-section">
                <div class="section-title">Controllers</div>
                <div v-if="controllers.length === 0" class="empty-state">No controllers</div>
                <div v-else class="node-grid">
                    <div class="node-card" v-for="c in controllers" :key="c.id"
                        @click="selectController(c.id)" style="cursor: pointer;">
                        <div class="node-card-header">
                            <span class="node-card-name">{{ c.hostname || shortId(c.id) }}</span>
                            <span class="badge" :class="'badge-' + c.status">{{ c.status }}</span>
                        </div>
                        <div class="node-card-meta">up {{ uptime(c.started_at) }}</div>
                    </div>
                </div>
            </div>

            <div class="dashboard-section">
                <div class="section-title">Recent Jobs</div>
                <div v-if="recentJobs.length === 0" class="empty-state">No jobs yet</div>
                <div v-else class="card-grid">
                    <div class="card" v-for="job in recentJobs" :key="job.id"
                        @click="selectJob(job.id)">
                        <div class="card-header">
                            <span class="card-id">{{ shortId(job.id) }}</span>
                            <span class="badge" :class="'badge-' + job.status">
                                {{ job.status }}
                            </span>
                        </div>
                        <div class="card-body">
                            <div class="card-row">
                                <span class="label">Target</span>
                                <span class="mono">{{ job.target.scope }}{{ job.target.value ? ':' + job.target.value : '' }}</span>
                            </div>
                            <div class="card-row">
                                <span class="label">Steps</span>
                                <span class="mono">{{ jobProgress(job).done }}/{{ jobProgress(job).total }}</span>
                            </div>
                            <div class="progress-bar">
                                <div class="progress-fill" :class="statusColor(job.status)"
                                    :style="{ width: progressPct(job) + '%' }"></div>
                            </div>
                            <div class="card-row text-dim" style="font-size: 12px;">
                                {{ timeAgo(job.created_at) }}
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="dashboard-section">
                <div class="section-title">Nodes</div>
                <div v-if="nodes.length === 0" class="empty-state">No nodes registered</div>
                <div v-else class="node-grid">
                    <div class="node-card" v-for="node in nodes" :key="node.id"
                        @click="selectNode(node.id)" style="cursor: pointer;">
                        <div class="node-card-header">
                            <span class="node-card-name">{{ node.hostname || shortId(node.id) }}</span>
                            <span class="badge" :class="'badge-' + node.status">{{ node.status }}</span>
                        </div>
                        <div class="node-card-groups">
                            <span class="group-badge" v-for="g in node.groups" :key="g">{{ g }}</span>
                        </div>
                        <div class="node-card-meta">{{ timeAgo(node.last_seen) }}</div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Jobs View -->
        <div v-if="view === 'jobs'">
            <div class="filter-bar">
                <div class="filter-pills">
                    <button class="filter-pill" :class="{ active: jobFilter === 'all' }"
                        @click="jobFilter = 'all'">All</button>
                    <button class="filter-pill" :class="{ active: jobFilter === 'pending' }"
                        @click="jobFilter = 'pending'">Pending</button>
                    <button class="filter-pill" :class="{ active: jobFilter === 'running' }"
                        @click="jobFilter = 'running'">Running</button>
                    <button class="filter-pill" :class="{ active: jobFilter === 'completed' }"
                        @click="jobFilter = 'completed'">Completed</button>
                    <button class="filter-pill" :class="{ active: jobFilter === 'failed' }"
                        @click="jobFilter = 'failed'">Failed</button>
                    <button class="filter-pill" :class="{ active: jobFilter === 'cancelled' }"
                        @click="jobFilter = 'cancelled'">Cancelled</button>
                </div>
                <button class="btn btn-sm" @click="openSubmitForm">New Job</button>
            </div>

            <div v-if="filteredJobs.length === 0" class="empty-state">No jobs match this filter</div>
            <div v-else class="card-grid">
                <div class="card" v-for="job in filteredJobs" :key="job.id"
                    @click="selectJob(job.id)">
                    <div class="card-header">
                        <span class="card-id">{{ shortId(job.id) }}</span>
                        <span class="badge" :class="'badge-' + job.status">
                            {{ job.status }}
                        </span>
                    </div>
                    <div class="card-body">
                        <div class="card-row">
                            <span class="label">Target</span>
                            <span class="mono">{{ job.target.scope }}{{ job.target.value ? ':' + job.target.value : '' }}</span>
                        </div>
                        <div class="card-row">
                            <span class="label">Strategy</span>
                            <span class="mono">{{ job.strategy }}</span>
                        </div>
                        <div class="card-row">
                            <span class="label">Steps</span>
                            <span class="mono">{{ jobProgress(job).done }}/{{ jobProgress(job).total }}</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" :class="statusColor(job.status)"
                                :style="{ width: progressPct(job) + '%' }"></div>
                        </div>
                        <div class="card-row text-dim" style="font-size: 12px;">
                            {{ timeAgo(job.created_at) }}
                            <span v-if="job.owner" class="mono">owner: {{ shortId(job.owner) }}</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Cluster View -->
        <div v-if="view === 'cluster'">
            <div class="dashboard-section">
                <div class="section-title">Controllers</div>
                <div v-if="controllers.length === 0" class="empty-state">No controllers</div>
                <div v-else class="table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th>ID</th>
                                <th>Hostname</th>
                                <th>Status</th>
                                <th>Started</th>
                                <th>Last Seen</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="c in controllers" :key="c.id"
                                @click="selectController(c.id)" style="cursor: pointer;">
                                <td class="mono">{{ shortId(c.id) }}</td>
                                <td>{{ c.hostname }}</td>
                                <td><span class="badge" :class="'badge-' + c.status">{{ c.status }}</span></td>
                                <td class="text-dim">{{ timeAgo(c.started_at) }}</td>
                                <td class="text-dim">{{ timeAgo(c.last_seen) }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div class="dashboard-section">
                <div class="section-title">Nodes</div>
                <div v-if="nodes.length === 0" class="empty-state">No nodes registered</div>
                <div v-else class="table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th>ID</th>
                                <th>Hostname</th>
                                <th>Groups</th>
                                <th>Backends</th>
                                <th>Status</th>
                                <th>Last Seen</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="node in nodes" :key="node.id"
                                @click="selectNode(node.id)" style="cursor: pointer;">
                                <td class="mono">{{ shortId(node.id) }}</td>
                                <td>{{ node.hostname }}</td>
                                <td>
                                    <div class="group-badges">
                                        <span class="group-badge" v-for="g in node.groups" :key="g">{{ g }}</span>
                                    </div>
                                </td>
                                <td class="mono text-dim">{{ (node.backends || []).join(', ') }}</td>
                                <td><span class="badge" :class="'badge-' + node.status">{{ node.status }}</span></td>
                                <td class="text-dim">{{ timeAgo(node.last_seen) }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </div>

    <!-- Job Detail Modal -->
    <div v-if="showJobModal && selectedJob" class="modal-overlay" @click.self="closeJobModal">
        <div class="modal">
            <div class="modal-header">
                <h2>
                    <span class="badge" :class="'badge-' + selectedJob.status" style="margin-right: 8px;">
                        {{ selectedJob.status }}
                    </span>
                    Job {{ shortId(selectedJob.id) }}
                </h2>
                <button class="modal-close" @click="closeJobModal">&times;</button>
            </div>
            <div class="modal-body">
                <div class="job-meta">
                    <div class="meta-item">
                        <span class="label">ID</span>
                        <span class="value">{{ selectedJob.id }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Target</span>
                        <span class="value">{{ selectedJob.target.scope }}{{ selectedJob.target.value ? ':' + selectedJob.target.value : '' }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Strategy</span>
                        <span class="value">{{ selectedJob.strategy }}</span>
                    </div>
                    <div class="meta-item" v-if="selectedJob.owner">
                        <span class="label">Owner</span>
                        <span class="value">{{ shortId(selectedJob.owner) }}</span>
                    </div>
                    <div class="meta-item" v-if="selectedJob.timeout">
                        <span class="label">Timeout</span>
                        <span class="value">{{ selectedJob.timeout }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Created</span>
                        <span class="value">{{ timeAgo(selectedJob.created_at) }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Updated</span>
                        <span class="value">{{ timeAgo(selectedJob.updated_at) }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Nodes</span>
                        <span class="value">{{ (selectedJob.expected || []).length }}</span>
                    </div>
                </div>

                <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 16px;">
                    <button v-if="selectedJob.status === 'running' || selectedJob.status === 'pending'"
                        class="btn btn-danger btn-sm" @click="cancelJob(selectedJob.id)">Cancel Job</button>
                    <button class="btn btn-ghost btn-sm" @click="showRawJson = !showRawJson">
                        {{ showRawJson ? 'Phase Tree' : 'Raw JSON' }}
                    </button>
                </div>

                <div v-if="showRawJson" class="raw-json">{{ JSON.stringify(selectedJob, null, 2) }}</div>

                <div v-else class="phase-tree">
                    <div class="section-title">Phases</div>
                    <div v-for="(phase, idx) in selectedJob.tasks" :key="idx" class="phase-node">
                        <!-- Leaf phase -->
                        <div v-if="phase.backend" class="phase-leaf">
                            <div class="phase-header">
                                <span class="phase-index">{{ leafIndex(selectedJob.tasks, idx) }}</span>
                                <span class="phase-name">{{ phase.backend }}.{{ phase.action }}</span>
                                <span v-if="phase.condition && phase.condition !== 'always'" class="phase-condition">
                                    {{ phase.condition }}
                                </span>
                                <span v-if="phase.timeout" class="text-dim mono" style="font-size: 11px;">
                                    timeout: {{ phase.timeout }}
                                </span>
                                <span v-if="phase.max_retries" class="text-dim mono" style="font-size: 11px;">
                                    retries: {{ phase.max_retries }}
                                </span>
                            </div>
                            <div v-if="formatParams(phase.params)" class="phase-params">
                                {{ formatParams(phase.params) }}
                            </div>
                            <div class="node-results" v-if="getStepResults(leafIndex(selectedJob.tasks, idx))">
                                <div class="node-result" v-for="(result, nodeId) in getStepResults(leafIndex(selectedJob.tasks, idx))" :key="nodeId">
                                    <span class="node-id">{{ shortId(nodeId) }}</span>
                                    <div class="result-info">
                                        <div class="result-meta">
                                            <span class="badge badge-sm" :class="'badge-' + result.status">{{ result.status }}</span>
                                            <span class="duration">{{ result.duration }}</span>
                                            <span v-if="result.attempts > 1" class="attempts-badge">
                                                attempt {{ result.attempts }}
                                            </span>
                                        </div>
                                        <div v-if="result.output" class="result-output">{{ result.output }}</div>
                                        <div v-if="result.error" class="result-error">{{ result.error }}</div>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <!-- Branch phase (pipeline) -->
                        <div v-else-if="phase.tasks && phase.tasks.length > 0" class="phase-branch">
                            <div class="phase-branch-header" @click="toggleBranch(idx)">
                                <span class="toggle">{{ expandedBranches[idx] ? '\u25BC' : '\u25B6' }}</span>
                                <span>{{ phaseLabel(phase) }}</span>
                                <span v-if="phase.condition && phase.condition !== 'always'" class="phase-condition">
                                    {{ phase.condition }}
                                </span>
                            </div>
                            <div v-if="expandedBranches[idx]" class="phase-children">
                                <div v-for="(sub, subIdx) in phase.tasks" :key="subIdx" class="phase-node">
                                    <div class="phase-leaf">
                                        <div class="phase-header">
                                            <span class="phase-index">{{ branchLeafIndex(selectedJob.tasks, idx, subIdx) }}</span>
                                            <span class="phase-name">{{ sub.backend }}.{{ sub.action }}</span>
                                            <span v-if="sub.condition && sub.condition !== 'always'" class="phase-condition">
                                                {{ sub.condition }}
                                            </span>
                                        </div>
                                        <div v-if="formatParams(sub.params)" class="phase-params">
                                            {{ formatParams(sub.params) }}
                                        </div>
                                        <div class="node-results" v-if="getStepResults(branchLeafIndex(selectedJob.tasks, idx, subIdx))">
                                            <div class="node-result" v-for="(result, nodeId) in getStepResults(branchLeafIndex(selectedJob.tasks, idx, subIdx))" :key="nodeId">
                                                <span class="node-id">{{ shortId(nodeId) }}</span>
                                                <div class="result-info">
                                                    <div class="result-meta">
                                                        <span class="badge badge-sm" :class="'badge-' + result.status">{{ result.status }}</span>
                                                        <span class="duration">{{ result.duration }}</span>
                                                        <span v-if="result.attempts > 1" class="attempts-badge">
                                                            attempt {{ result.attempts }}
                                                        </span>
                                                    </div>
                                                    <div v-if="result.output" class="result-output">{{ result.output }}</div>
                                                    <div v-if="result.error" class="result-error">{{ result.error }}</div>
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <!-- Controller Detail Modal -->
    <div v-if="showControllerModal && selectedController" class="modal-overlay" @click.self="closeControllerModal">
        <div class="modal">
            <div class="modal-header">
                <h2>
                    <span class="badge" :class="'badge-' + selectedController.status" style="margin-right: 8px;">
                        {{ selectedController.status }}
                    </span>
                    {{ selectedController.hostname || shortId(selectedController.id) }}
                </h2>
                <button class="modal-close" @click="closeControllerModal">&times;</button>
            </div>
            <div class="modal-body">
                <div class="job-meta">
                    <div class="meta-item">
                        <span class="label">ID</span>
                        <span class="value">{{ selectedController.id }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Hostname</span>
                        <span class="value">{{ selectedController.hostname }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Status</span>
                        <span class="value">{{ selectedController.status }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Uptime</span>
                        <span class="value">{{ uptime(selectedController.started_at) }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Started</span>
                        <span class="value">{{ formatTime(selectedController.started_at) }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Last Seen</span>
                        <span class="value">{{ timeAgo(selectedController.last_seen) }}</span>
                    </div>
                </div>

                <div v-if="natsInfo" class="dashboard-section">
                    <div class="section-title">NATS Server</div>
                    <div class="job-meta">
                        <div class="meta-item">
                            <span class="label">Server Name</span>
                            <span class="value">{{ natsInfo.server_name }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Version</span>
                            <span class="value">{{ natsInfo.version }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Uptime</span>
                            <span class="value">{{ natsInfo.uptime }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">CPU</span>
                            <span class="value">{{ natsInfo.cpu.toFixed(1) }}%</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Memory</span>
                            <span class="value">{{ formatBytes(natsInfo.mem) }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Cores</span>
                            <span class="value">{{ natsInfo.cores }}</span>
                        </div>
                    </div>
                    <div class="job-meta mt-16">
                        <div class="meta-item">
                            <span class="label">Connections</span>
                            <span class="value">{{ natsInfo.connections }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Total Connections</span>
                            <span class="value">{{ natsInfo.total_connections }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Routes</span>
                            <span class="value">{{ natsInfo.routes }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Subscriptions</span>
                            <span class="value">{{ natsInfo.subscriptions }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Slow Consumers</span>
                            <span class="value">{{ natsInfo.slow_consumers }}</span>
                        </div>
                    </div>
                    <div class="job-meta mt-16">
                        <div class="meta-item">
                            <span class="label">Messages In</span>
                            <span class="value">{{ formatNumber(natsInfo.in_msgs) }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Messages Out</span>
                            <span class="value">{{ formatNumber(natsInfo.out_msgs) }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Data In</span>
                            <span class="value">{{ formatBytes(natsInfo.in_bytes) }}</span>
                        </div>
                        <div class="meta-item">
                            <span class="label">Data Out</span>
                            <span class="value">{{ formatBytes(natsInfo.out_bytes) }}</span>
                        </div>
                    </div>
                </div>

                <div class="dashboard-section">
                    <div class="section-title">Owned Jobs</div>
                    <div v-if="controllerJobs.length === 0" class="text-dim" style="font-size: 13px;">No jobs owned by this controller</div>
                    <div v-else>
                        <div v-for="job in controllerJobs" :key="job.id" class="card" style="margin-bottom: 8px;"
                            @click="closeControllerModal(); selectJob(job.id);">
                            <div class="card-header">
                                <span class="card-id">{{ shortId(job.id) }}</span>
                                <span class="badge" :class="'badge-' + job.status">{{ job.status }}</span>
                            </div>
                            <div class="card-body">
                                <div class="card-row">
                                    <span class="label">Target</span>
                                    <span class="mono">{{ job.target.scope }}{{ job.target.value ? ':' + job.target.value : '' }}</span>
                                </div>
                                <div class="card-row text-dim" style="font-size: 12px;">
                                    {{ timeAgo(job.created_at) }}
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <!-- Node Detail Modal -->
    <div v-if="showNodeModal && selectedNode" class="modal-overlay" @click.self="closeNodeModal">
        <div class="modal">
            <div class="modal-header">
                <h2>
                    <span class="badge" :class="'badge-' + selectedNode.status" style="margin-right: 8px;">
                        {{ selectedNode.status }}
                    </span>
                    {{ selectedNode.hostname || shortId(selectedNode.id) }}
                </h2>
                <button class="modal-close" @click="closeNodeModal">&times;</button>
            </div>
            <div class="modal-body">
                <div class="job-meta">
                    <div class="meta-item">
                        <span class="label">ID</span>
                        <span class="value">{{ selectedNode.id }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Hostname</span>
                        <span class="value">{{ selectedNode.hostname }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Status</span>
                        <span class="value">{{ selectedNode.status }}</span>
                    </div>
                    <div class="meta-item">
                        <span class="label">Last Seen</span>
                        <span class="value">{{ timeAgo(selectedNode.last_seen) }}</span>
                    </div>
                </div>

                <div class="dashboard-section">
                    <div class="section-title">Groups</div>
                    <div v-if="!selectedNode.groups || selectedNode.groups.length === 0" class="text-dim" style="font-size: 13px;">No groups</div>
                    <div v-else class="group-badges">
                        <span class="group-badge" v-for="g in selectedNode.groups" :key="g">{{ g }}</span>
                    </div>
                </div>

                <div class="dashboard-section">
                    <div class="section-title">Backends</div>
                    <div v-if="!selectedNode.backends || selectedNode.backends.length === 0" class="text-dim" style="font-size: 13px;">No backends</div>
                    <div v-else class="group-badges">
                        <span class="group-badge" v-for="b in selectedNode.backends" :key="b">{{ b }}</span>
                    </div>
                </div>

                <div class="dashboard-section">
                    <div class="section-title">Recent Jobs</div>
                    <div v-if="nodeJobs.length === 0" class="text-dim" style="font-size: 13px;">No jobs have targeted this node</div>
                    <div v-else>
                        <div v-for="job in nodeJobs" :key="job.id" class="card" style="margin-bottom: 8px;"
                            @click="closeNodeModal(); selectJob(job.id);">
                            <div class="card-header">
                                <span class="card-id">{{ shortId(job.id) }}</span>
                                <span class="badge" :class="'badge-' + job.status">{{ job.status }}</span>
                            </div>
                            <div class="card-body">
                                <div class="card-row">
                                    <span class="label">Target</span>
                                    <span class="mono">{{ job.target.scope }}{{ job.target.value ? ':' + job.target.value : '' }}</span>
                                </div>
                                <div class="card-row text-dim" style="font-size: 12px;">
                                    {{ timeAgo(job.created_at) }}
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <!-- Job Submit Modal -->
    <div v-if="showSubmitForm" class="modal-overlay" @click.self="closeSubmitForm">
        <div class="modal">
            <div class="modal-header">
                <h2>Submit Job</h2>
                <button class="modal-close" @click="closeSubmitForm">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-row">
                    <div class="form-group">
                        <label>Target Scope</label>
                        <select v-model="submitForm.target.scope">
                            <option value="all">All Nodes</option>
                            <option value="group">Group</option>
                            <option value="node">Node</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Target Value</label>
                        <input v-model="submitForm.target.value" type="text"
                            :placeholder="submitForm.target.scope === 'all' ? '(not needed)' : submitForm.target.scope === 'group' ? 'group name' : 'node ID'"
                            :disabled="submitForm.target.scope === 'all'">
                    </div>
                </div>

                <div class="form-row">
                    <div class="form-group">
                        <label>Strategy</label>
                        <div class="radio-group">
                            <label>
                                <input type="radio" v-model="submitForm.strategy" value="fail-fast"> fail-fast
                            </label>
                            <label>
                                <input type="radio" v-model="submitForm.strategy" value="continue"> continue
                            </label>
                        </div>
                    </div>
                    <div class="form-group">
                        <label>Timeout</label>
                        <input v-model="submitForm.timeout" type="text" placeholder="e.g. 30m (optional)">
                    </div>
                </div>

                <div class="form-group">
                    <label>Tasks</label>
                    <div v-for="(task, idx) in submitForm.tasks" :key="idx" class="task-row">
                        <div class="task-row-header">
                            <span class="task-num">Task {{ idx + 1 }}</span>
                            <button v-if="submitForm.tasks.length > 1" class="remove-task-btn"
                                @click="removeTask(idx)">&times;</button>
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label>Backend</label>
                                <input v-model="task.backend" type="text" placeholder="e.g. apt, systemd, rke2">
                            </div>
                            <div class="form-group">
                                <label>Action</label>
                                <input v-model="task.action" type="text" placeholder="e.g. install, start, ping">
                            </div>
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label>Params (key=value, one per line)</label>
                                <textarea v-model="task.paramsText" rows="2"
                                    placeholder="package=nginx&#10;version=latest"></textarea>
                            </div>
                            <div class="form-group">
                                <label>Condition</label>
                                <select v-model="task.condition">
                                    <option value="">always (default)</option>
                                    <option value="on_success">on_success</option>
                                    <option value="on_failure">on_failure</option>
                                </select>
                            </div>
                        </div>
                    </div>
                    <button class="btn btn-ghost btn-sm mt-8" @click="addTask">+ Add Task</button>
                </div>

                <div class="form-actions">
                    <button class="btn btn-ghost" @click="closeSubmitForm">Cancel</button>
                    <button class="btn" @click="submitJob" :disabled="submitting">
                        {{ submitting ? 'Submitting...' : 'Submit Job' }}
                    </button>
                </div>
            </div>
        </div>
    </div>
    `,

    data() {
        return {
            view: 'dashboard',
            controllers: [],
            nodes: [],
            jobs: [],
            selectedJob: null,
            selectedNode: null,
            selectedController: null,
            natsInfo: null,
            showJobModal: false,
            showNodeModal: false,
            showControllerModal: false,
            showSubmitForm: false,
            showRawJson: false,
            jobFilter: 'all',
            error: null,
            errorTimer: null,
            expandedBranches: {},
            submitting: false,
            submitForm: this.emptyForm(),

            // Polling intervals
            _controllerInterval: null,
            _nodeInterval: null,
            _jobInterval: null,
            _detailInterval: null,
        };
    },

    computed: {
        onlineNodes() {
            return this.nodes.filter(n => n.status === 'online').length;
        },
        activeJobs() {
            return this.jobs.filter(j => j.status === 'running' || j.status === 'pending').length;
        },
        failedJobs() {
            return this.jobs.filter(j => j.status === 'failed').length;
        },
        recentJobs() {
            const sorted = [...this.jobs].sort((a, b) => {
                // Running/pending first
                const aActive = a.status === 'running' || a.status === 'pending' ? 0 : 1;
                const bActive = b.status === 'running' || b.status === 'pending' ? 0 : 1;
                if (aActive !== bActive) return aActive - bActive;
                // Then by creation time (newest first)
                return new Date(b.created_at) - new Date(a.created_at);
            });
            return sorted.slice(0, 10);
        },
        filteredJobs() {
            let list = [...this.jobs];
            if (this.jobFilter !== 'all') {
                list = list.filter(j => j.status === this.jobFilter);
            }
            return list.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
        },
        nodeJobs() {
            if (!this.selectedNode) return [];
            const nodeId = this.selectedNode.id;
            return this.jobs
                .filter(j => j.expected && j.expected.includes(nodeId))
                .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
                .slice(0, 10);
        },
        controllerJobs() {
            if (!this.selectedController) return [];
            const ctrlId = this.selectedController.id;
            return this.jobs
                .filter(j => j.owner === ctrlId)
                .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
                .slice(0, 10);
        },
    },

    methods: {
        // Helpers exposed to template
        timeAgo,
        shortId,
        statusColor,
        statusIcon,
        flatLeafCount,
        phaseLabel,
        formatParams,
        jobProgress,
        uptime,
        formatBytes,
        formatNumber,
        formatTime,

        progressPct(job) {
            const { done, total } = jobProgress(job);
            if (total === 0) return 0;
            // For completed/failed/cancelled, show full bar
            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') return 100;
            return Math.round((done / total) * 100);
        },

        // Leaf indexing for phase tree display
        leafIndex(phases, upToIdx) {
            let idx = 0;
            for (let i = 0; i < upToIdx; i++) {
                const p = phases[i];
                if (p.backend) {
                    idx++;
                } else if (p.tasks) {
                    idx += flatLeafCount(p.tasks);
                }
            }
            return idx;
        },

        branchLeafIndex(phases, branchIdx, subIdx) {
            let idx = this.leafIndex(phases, branchIdx);
            const branch = phases[branchIdx];
            for (let i = 0; i < subIdx; i++) {
                const sub = branch.tasks[i];
                if (sub.backend) {
                    idx++;
                } else if (sub.tasks) {
                    idx += flatLeafCount(sub.tasks);
                }
            }
            return idx;
        },

        getStepResults(stepIdx) {
            if (!this.selectedJob || !this.selectedJob.results) return null;
            return this.selectedJob.results[String(stepIdx)] || null;
        },

        toggleBranch(idx) {
            this.expandedBranches[idx] = !this.expandedBranches[idx];
        },

        // API calls
        async fetchControllers() {
            try {
                this.controllers = await api.get('/controllers');
            } catch (e) {
                this.showError('Controllers: ' + e.message);
            }
        },

        async fetchNodes() {
            try {
                this.nodes = await api.get('/nodes');
            } catch (e) {
                this.showError('Nodes: ' + e.message);
            }
        },

        async fetchJobs() {
            try {
                this.jobs = await api.get('/jobs');
            } catch (e) {
                this.showError('Jobs: ' + e.message);
            }
        },

        async fetchSelectedJob() {
            if (!this.selectedJob) return;
            try {
                this.selectedJob = await api.get('/job/' + this.selectedJob.id);
            } catch (e) {
                this.showError('Job detail: ' + e.message);
            }
        },

        async selectJob(id) {
            try {
                this.selectedJob = await api.get('/job/' + id);
                this.showJobModal = true;
                this.showRawJson = false;
                this.expandedBranches = {};
                // Expand all branches by default
                if (this.selectedJob.tasks) {
                    this.selectedJob.tasks.forEach((p, i) => {
                        if (p.tasks && p.tasks.length > 0) {
                            this.expandedBranches[i] = true;
                        }
                    });
                }
                this.startDetailPolling();
            } catch (e) {
                this.showError('Failed to load job: ' + e.message);
            }
        },

        closeJobModal() {
            this.showJobModal = false;
            this.selectedJob = null;
            this.stopDetailPolling();
        },

        async selectNode(id) {
            try {
                this.selectedNode = await api.get('/node/' + id);
                this.showNodeModal = true;
            } catch (e) {
                this.showError('Failed to load node: ' + e.message);
            }
        },

        closeNodeModal() {
            this.showNodeModal = false;
            this.selectedNode = null;
        },

        selectController(id) {
            const ctrl = this.controllers.find(c => c.id === id);
            if (!ctrl) {
                this.showError('Controller not found');
                return;
            }
            this.selectedController = ctrl;
            this.showControllerModal = true;
            this.natsInfo = null;
            this.fetchNatsInfo();
        },

        closeControllerModal() {
            this.showControllerModal = false;
            this.selectedController = null;
            this.natsInfo = null;
        },

        async fetchNatsInfo() {
            try {
                this.natsInfo = await api.get('/nats');
            } catch (e) {
                // NATS info not available (e.g. different controller) — not an error
            }
        },

        async cancelJob(id) {
            try {
                await api.post('/job/' + id + '/cancel');
                await this.fetchSelectedJob();
                await this.fetchJobs();
            } catch (e) {
                this.showError('Cancel: ' + e.message);
            }
        },

        // Submit form
        emptyForm() {
            return {
                target: { scope: 'all', value: '' },
                strategy: 'fail-fast',
                timeout: '',
                tasks: [{ backend: '', action: '', paramsText: '', condition: '' }],
            };
        },

        openSubmitForm() {
            this.submitForm = this.emptyForm();
            this.showSubmitForm = true;
        },

        closeSubmitForm() {
            this.showSubmitForm = false;
        },

        addTask() {
            this.submitForm.tasks.push({ backend: '', action: '', paramsText: '', condition: '' });
        },

        removeTask(idx) {
            this.submitForm.tasks.splice(idx, 1);
        },

        parseParams(text) {
            if (!text || !text.trim()) return {};
            const params = {};
            text.trim().split('\n').forEach(line => {
                const eq = line.indexOf('=');
                if (eq > 0) {
                    params[line.substring(0, eq).trim()] = line.substring(eq + 1).trim();
                }
            });
            return params;
        },

        async submitJob() {
            this.submitting = true;
            try {
                const body = {
                    target: {
                        scope: this.submitForm.target.scope,
                        value: this.submitForm.target.scope === 'all' ? '' : this.submitForm.target.value,
                    },
                    strategy: this.submitForm.strategy,
                    tasks: this.submitForm.tasks.map(t => {
                        const phase = {
                            backend: t.backend,
                            action: t.action,
                            params: this.parseParams(t.paramsText),
                        };
                        if (t.condition) phase.condition = t.condition;
                        return phase;
                    }),
                };
                if (this.submitForm.timeout) {
                    body.timeout = this.submitForm.timeout;
                }
                const job = await api.post('/job', body);
                this.closeSubmitForm();
                await this.fetchJobs();
                await this.selectJob(job.id);
            } catch (e) {
                this.showError('Submit: ' + e.message);
            } finally {
                this.submitting = false;
            }
        },

        // Polling
        startPolling() {
            this._controllerInterval = setInterval(() => this.fetchControllers(), 15000);
            this._nodeInterval = setInterval(() => this.fetchNodes(), 10000);
            this._jobInterval = setInterval(() => this.fetchJobs(), 3000);
        },

        stopPolling() {
            clearInterval(this._controllerInterval);
            clearInterval(this._nodeInterval);
            clearInterval(this._jobInterval);
        },

        startDetailPolling() {
            this.stopDetailPolling();
            this._detailInterval = setInterval(() => this.fetchSelectedJob(), 1000);
        },

        stopDetailPolling() {
            clearInterval(this._detailInterval);
            this._detailInterval = null;
        },

        handleVisibility() {
            if (document.hidden) {
                this.stopPolling();
                this.stopDetailPolling();
            } else {
                this.fetchAll();
                this.startPolling();
                if (this.showJobModal && this.selectedJob) {
                    this.startDetailPolling();
                }
            }
        },

        async fetchAll() {
            await Promise.all([
                this.fetchControllers(),
                this.fetchNodes(),
                this.fetchJobs(),
            ]);
        },

        showError(msg) {
            this.error = msg;
            if (this.errorTimer) clearTimeout(this.errorTimer);
            this.errorTimer = setTimeout(() => { this.error = null; }, 5000);
        },
    },

    mounted() {
        this.fetchAll();
        this.startPolling();
        document.addEventListener('visibilitychange', this.handleVisibility);
    },

    beforeUnmount() {
        this.stopPolling();
        this.stopDetailPolling();
        document.removeEventListener('visibilitychange', this.handleVisibility);
    },
});

app.mount('#app');
