export const features = [
  {
    icon: 'Zap',
    title: 'Zero Dependencies',
    description: 'Pure Go standard library. No external packages, no supply chain risks, no version conflicts.',
    badge: 'Core',
    highlight: true,
    span: 2,
  },
  {
    icon: 'Lock',
    title: 'Automatic TLS',
    description: "Let's Encrypt HTTP-01 challenge built-in. HTTPS is live the moment your container starts.",
    badge: 'Security',
    highlight: true,
  },
  {
    icon: 'Tag',
    title: 'Label-Based Config',
    description: 'No config files. Add Docker labels to your containers and DockRouter handles the rest.',
    badge: 'Core',
    highlight: true,
  },
  {
    icon: 'Package',
    title: 'Single Binary',
    description: 'Under 10MB. Download, run, done. No runtime dependencies, no installation steps.',
    badge: '<10MB',
    highlight: true,
    span: 2,
  },
  {
    icon: 'RefreshCw',
    title: 'Hot Reload',
    description: 'Routes update instantly when containers start or stop. Zero downtime, zero intervention.',
  },
  {
    icon: 'LayoutDashboard',
    title: 'Built-in Dashboard',
    description: 'Real-time admin UI on port 9090. Monitor routes, containers, and certificates at a glance.',
  },
  {
    icon: 'Cable',
    title: 'WebSocket Support',
    description: 'Transparent WebSocket proxying with connection upgrades. No extra configuration needed.',
  },
  {
    icon: 'BarChart3',
    title: 'Prometheus Metrics',
    description: 'Built-in /metrics endpoint. Track requests, latency, errors, and active connections.',
  },
  {
    icon: 'Scale',
    title: 'Load Balancing',
    description: 'Four strategies: round-robin, weighted, IP hash, and least connections. Per-route config.',
  },
  {
    icon: 'Shield',
    title: 'Rate Limiting',
    description: 'Token bucket per IP, per header, or per route. Configurable windows and burst limits.',
  },
  {
    icon: 'CircuitBoard',
    title: 'Circuit Breaker',
    description: 'Automatic failure detection. Open, half-open, closed states protect your services.',
  },
  {
    icon: 'ShieldCheck',
    title: 'Security Headers',
    description: 'HSTS, CORS, X-Frame-Options, CSP. IP whitelisting and blacklisting with CIDR support.',
  },
]
