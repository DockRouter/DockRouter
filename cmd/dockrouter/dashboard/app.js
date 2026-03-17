// DockRouter Admin Dashboard

const API_BASE = '/api/v1';
let eventSource = null;

// Initialize on load
document.addEventListener('DOMContentLoaded', () => {
    loadOverview();
    loadRoutes();
    loadCertificates();
    loadContainers();
    connectSSE();
});

// Fetch helper
async function fetchAPI(endpoint) {
    const response = await fetch(API_BASE + endpoint);
    return response.json();
}

// Create element helper (XSS safe)
function createElement(tag, attrs = {}, children = []) {
    const el = document.createElement(tag);
    for (const [key, value] of Object.entries(attrs)) {
        if (key === 'className') {
            el.className = value;
        } else {
            el.setAttribute(key, value);
        }
    }
    for (const child of children) {
        if (typeof child === 'string') {
            el.textContent = child;
        } else {
            el.appendChild(child);
        }
    }
    return el;
}

// Load overview stats
async function loadOverview() {
    try {
        const status = await fetchAPI('/status');
        document.getElementById('routes-count').textContent = status.routes || 0;
        document.getElementById('containers-count').textContent = status.containers || 0;
        document.getElementById('certs-count').textContent = status.certificates || 0;
        document.getElementById('uptime').textContent = formatUptime(status.uptime);
    } catch (err) {
        console.error('Failed to load overview:', err);
    }
}

// Load routes table
async function loadRoutes() {
    try {
        const routes = await fetchAPI('/routes');
        const tbody = document.querySelector('#routes-table tbody');
        tbody.textContent = '';

        routes.forEach(route => {
            const codeEl = createElement('code', {}, [route.host]);
            const td1 = createElement('td', {}, [codeEl]);
            const td2 = createElement('td', {}, [route.path_prefix || '/']);
            const td3 = createElement('td', {}, [route.backend || '-']);
            const td4 = createElement('td', {}, [route.tls ? '✓' : '—']);
            const statusClass = route.healthy ? 'healthy' : 'unhealthy';
            const statusText = route.healthy ? 'Healthy' : 'Unhealthy';
            const statusEl = createElement('span', { className: 'status ' + statusClass }, [statusText]);
            const td5 = createElement('td', {}, [statusEl]);
            const tr = createElement('tr', {}, [td1, td2, td3, td4, td5]);
            tbody.appendChild(tr);
        });
    } catch (err) {
        console.error('Failed to load routes:', err);
    }
}

// Load certificates table
async function loadCertificates() {
    try {
        const certs = await fetchAPI('/certificates');
        const tbody = document.querySelector('#certs-table tbody');
        tbody.textContent = '';

        certs.forEach(cert => {
            const expiry = new Date(cert.expiry * 1000);
            const daysLeft = Math.ceil((expiry - new Date()) / 86400000);
            const domainEl = createElement('code', {}, [cert.domain]);
            const td1 = createElement('td', {}, [domainEl]);
            const td2 = createElement('td', {}, [cert.issuer || "Let's Encrypt"]);
            const td3 = createElement('td', {}, [expiry.toLocaleDateString()]);
            const statusClass = daysLeft > 30 ? 'healthy' : 'warning';
            const statusEl = createElement('span', { className: 'status ' + statusClass }, [daysLeft + ' days']);
            const td4 = createElement('td', {}, [statusEl]);
            const tr = createElement('tr', {}, [td1, td2, td3, td4]);
            tbody.appendChild(tr);
        });
    } catch (err) {
        console.error('Failed to load certificates:', err);
    }
}

// Load containers table
async function loadContainers() {
    try {
        const containers = await fetchAPI('/containers');
        const tbody = document.querySelector('#containers-table tbody');
        tbody.textContent = '';

        containers.forEach(container => {
            const nameEl = createElement('code', {}, [container.name]);
            const td1 = createElement('td', {}, [nameEl]);
            const td2 = createElement('td', {}, [container.image]);
            const statusClass = container.running ? 'healthy' : 'unhealthy';
            const statusEl = createElement('span', { className: 'status ' + statusClass }, [container.status]);
            const td3 = createElement('td', {}, [statusEl]);
            const drLabels = Object.keys(container.labels || {}).filter(l => l.startsWith('dr.')).length;
            const td4 = createElement('td', {}, [drLabels + ' labels']);
            const tr = createElement('tr', {}, [td1, td2, td3, td4]);
            tbody.appendChild(tr);
        });
    } catch (err) {
        console.error('Failed to load containers:', err);
    }
}

// Connect to SSE for real-time updates
function connectSSE() {
    if (eventSource) {
        eventSource.close();
    }

    eventSource = new EventSource(API_BASE + '/events');

    eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            handleEvent(data);
        } catch (err) {
            console.error('Failed to parse SSE event:', err);
        }
    };

    eventSource.onerror = () => {
        console.log('SSE connection lost, reconnecting...');
        setTimeout(connectSSE, 5000);
    };
}

// Handle SSE events
function handleEvent(event) {
    console.log('Event:', event.type, event.data);

    switch (event.type) {
        case 'route.added':
        case 'route.removed':
            loadRoutes();
            break;
        case 'certificate.issued':
        case 'certificate.renewed':
            loadCertificates();
            break;
        case 'container.started':
        case 'container.stopped':
            loadContainers();
            loadOverview();
            break;
    }

    // Log to log section
    const logOutput = document.getElementById('log-output');
    const timestamp = new Date().toISOString();
    const existing = logOutput.textContent;
    logOutput.textContent = '[' + timestamp + '] ' + event.type + '\n' + existing;
}

// Format uptime - handles both string format ("40s", "1m30s") and seconds number
function formatUptime(uptime) {
    if (!uptime) return '-';

    // If it's a string, try to parse duration format like "40s", "1m30s", "2h15m30s"
    if (typeof uptime === 'string') {
        return uptime; // API already returns formatted duration
    }

    // If it's a number (seconds), format it
    const days = Math.floor(uptime / 86400);
    const hours = Math.floor((uptime % 86400) / 3600);
    if (days > 0) return days + 'd ' + hours + 'h';
    const minutes = Math.floor((uptime % 3600) / 60);
    if (hours > 0) return hours + 'h ' + minutes + 'm';
    return minutes + 'm';
}
