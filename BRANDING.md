# DockRouter — BRANDING.md

> Brand guidelines for DockRouter — the zero-dependency Docker-native ingress router.

---

## Brand Identity

### Name
**DockRouter** — one word, camelCase in prose, all lowercase in CLI/URLs.
- Prose: DockRouter
- CLI: `dockrouter`
- Domain: dockrouter.com
- GitHub: github.com/DockRouter
- Docker: `ghcr.io/dockrouter/dockrouter`

### Tagline
**"Route. Secure. Done."**

Alternative taglines:
- "Zero-config ingress for Docker"
- "Label it. Route it. Forget it."
- "Docker-native ingress that just works"

### Elevator Pitch
DockRouter is a single-binary, zero-dependency ingress router that automatically discovers Docker containers via labels and provisions TLS certificates — no YAML configs, no complex setup, just labels and go.

---

## Logo Concept

### Primary Mark
A **dock/harbor** icon merged with a **router/network** motif:
- Stylized anchor or bollard shape (dock)
- With network path lines branching out (router)
- Clean, geometric, minimal stroke weight

### Icon
Simplified version — a square/circle containing the dock-router fusion mark. Works at 16x16 favicon size.

### Text Logo
"DockRouter" in a monospace or semi-mono typeface, with "Dock" in the primary color and "Router" in a lighter weight or secondary color.

---

## Color Palette

### Primary Colors

| Name | Hex | Usage |
|------|-----|-------|
| **Dock Blue** | `#2563EB` | Primary brand, CTAs, links |
| **Dock Navy** | `#1E3A5F` | Headers, dark backgrounds |
| **Signal Orange** | `#F97316` | Accents, warnings, highlights |

### Neutrals

| Name | Hex | Usage |
|------|-----|-------|
| **Slate 900** | `#0F172A` | Dark mode background |
| **Slate 800** | `#1E293B` | Cards, panels |
| **Slate 400** | `#94A3B8` | Secondary text |
| **Slate 100** | `#F1F5F9` | Light mode background |
| **White** | `#FFFFFF` | Light surfaces, text on dark |

### Semantic Colors

| Name | Hex | Usage |
|------|-----|-------|
| **Healthy Green** | `#22C55E` | Health status, success |
| **Warning Amber** | `#EAB308` | Warnings, degraded |
| **Error Red** | `#EF4444` | Errors, unhealthy |
| **Info Blue** | `#3B82F6` | Info states |

---

## Typography

### Headings
**Inter** (or system-ui fallback) — Bold weight for hero text, Semibold for section headers.

### Body
**Inter** — Regular weight, 16px base size, 1.6 line height for readability.

### Code & Technical
**JetBrains Mono** (or `ui-monospace, monospace` fallback) — Used for all code blocks, CLI examples, label names, and the dashboard UI.

### Type Scale
- Hero: 48px / Bold
- H1: 36px / Bold
- H2: 24px / Semibold
- H3: 20px / Semibold
- Body: 16px / Regular
- Small: 14px / Regular
- Code: 14px / Mono Regular

---

## Voice & Tone

### Personality
- **Pragmatic** — No hype, no buzzwords. State what it does clearly.
- **Developer-first** — Speak to engineers who value simplicity.
- **Confident** — DockRouter is opinionated and proud of its zero-dep philosophy.
- **Concise** — Short sentences. Show code, not paragraphs.

### Writing Style
- Use active voice: "DockRouter discovers containers" not "Containers are discovered"
- Lead with value: "Auto-TLS in 3 lines" not "DockRouter has a feature that..."
- Code > words: Show `docker-compose.yml` examples over feature descriptions
- Avoid: "powerful", "enterprise-grade", "next-generation", "revolutionary"
- Prefer: "simple", "zero-config", "single binary", "just works"

### Example Copy

**Hero:**
> Stop configuring. Start shipping.  
> DockRouter discovers your containers, provisions TLS, and routes traffic — automatically.

**Feature highlight:**
> Add labels to your containers. DockRouter handles the rest.  
> ```yaml
> labels:
>   dr.enable: "true"
>   dr.host: "api.example.com"
>   dr.tls: "auto"
> ```
> That's it. HTTPS is live.

---

## Social Media

### GitHub
- Repository description: "Zero-dependency, single-binary Docker-native ingress router with automatic TLS."
- Topics: `docker`, `ingress`, `reverse-proxy`, `auto-tls`, `lets-encrypt`, `go`, `zero-dependency`

### X (Twitter)
- Handle: @DockRouter (if available)
- Bio: "Zero-config Docker ingress with auto-TLS. Single binary. No dependencies. Just labels."
- Content mix: 60% technical (releases, features), 30% education (Docker tips, TLS explainers), 10% community

### Developer Platforms
- Dev.to, Hashnode articles: "How I Built a Traefik Alternative in Go with Zero Dependencies"
- Hacker News: Focus on zero-dependency angle and Traefik comparison
- Reddit r/golang, r/docker, r/selfhosted: Community-first, helpful tone

---

## Dashboard Theme

The admin dashboard follows these design principles:
- Dark mode default (Slate 900 bg), light mode toggle
- Monospace accents for technical data
- Signal Orange for important state changes
- Healthy Green / Error Red for status indicators
- Minimal chrome — data-dense, no decorative elements
- Responsive: works on 1024px+ screens

---

## Comparison Positioning

### vs Traefik
"Traefik is great but complex. DockRouter is Traefik's simplicity promise, actually delivered. One binary, zero config files, zero dependencies."

### vs Caddy
"Caddy excels as a general web server. DockRouter is purpose-built for Docker — it discovers your containers and configures itself."

### vs Nginx Proxy Manager
"NPM needs a database, a UI to configure each host, and manual cert setup. DockRouter reads labels and does it all automatically."

### vs nginx + certbot
"If you enjoy writing nginx.conf and crontabs for certbot, you don't need DockRouter. Everyone else does."
