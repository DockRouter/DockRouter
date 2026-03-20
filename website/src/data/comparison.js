export const comparisonData = {
  headers: ['Feature', 'DockRouter', 'Traefik', 'Caddy', 'Nginx'],
  rows: [
    { feature: 'Zero dependencies', dockrouter: true, traefik: false, caddy: false, nginx: false },
    { feature: 'Single binary', dockrouter: true, traefik: true, caddy: true, nginx: false },
    { feature: 'Docker-native', dockrouter: true, traefik: true, caddy: false, nginx: false },
    { feature: 'Auto TLS', dockrouter: true, traefik: true, caddy: true, nginx: false },
    { feature: 'No config files', dockrouter: true, traefik: false, caddy: false, nginx: false },
    { feature: 'Label-based config', dockrouter: true, traefik: true, caddy: false, nginx: false },
    { feature: 'Built-in dashboard', dockrouter: true, traefik: true, caddy: false, nginx: false },
    { feature: '<10MB binary', dockrouter: true, traefik: false, caddy: false, nginx: false },
  ],
}
