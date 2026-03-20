import { ArrowRight, Github, Zap, Lock, Package } from 'lucide-react'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'

export function Hero() {
  return (
    <section className="relative min-h-screen flex items-center justify-center pt-16 overflow-hidden">
      {/* Background Effects */}
      <div className="absolute inset-0 dot-pattern opacity-50" />
      <div className="absolute top-1/4 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[800px] h-[600px] bg-dock-blue/5 rounded-full blur-[120px]" />
      <div className="absolute bottom-0 right-0 w-[400px] h-[400px] bg-signal-orange/5 rounded-full blur-[100px]" />

      <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-20">
        <div className="text-center max-w-4xl mx-auto">
          {/* Top Badge */}
          <div className="flex justify-center mb-8 animate-fade-in">
            <Badge variant="outline" className="px-4 py-1.5 text-sm gap-2">
              <span className="w-2 h-2 rounded-full bg-healthy animate-pulse" />
              v1.0 — Production Ready
            </Badge>
          </div>

          {/* Headline */}
          <h1 className="text-5xl sm:text-6xl lg:text-7xl font-extrabold tracking-tight leading-[1.1] mb-6 animate-fade-in-up" style={{ animationDelay: '0.1s' }}>
            Stop Configuring.
            <br />
            <span className="gradient-text">Start Shipping.</span>
          </h1>

          {/* Subtitle */}
          <p className="text-lg sm:text-xl text-[var(--text-secondary)] max-w-2xl mx-auto mb-10 leading-relaxed animate-fade-in-up" style={{ animationDelay: '0.2s' }}>
            DockRouter discovers your containers, provisions TLS, and routes
            traffic — automatically. One binary, zero config files.
          </p>

          {/* CTAs */}
          <div className="flex flex-col sm:flex-row items-center justify-center gap-4 mb-12 animate-fade-in-up" style={{ animationDelay: '0.3s' }}>
            <a href="#get-started">
              <Button size="lg" className="text-base px-8">
                Get Started
                <ArrowRight className="w-4 h-4" />
              </Button>
            </a>
            <a href="https://github.com/DockRouter/dockrouter" target="_blank" rel="noopener noreferrer">
              <Button variant="outline" size="lg" className="text-base px-8">
                <Github className="w-5 h-5" />
                View on GitHub
              </Button>
            </a>
          </div>

          {/* Feature Pills */}
          <div className="flex flex-wrap items-center justify-center gap-3 mb-16 animate-fade-in-up" style={{ animationDelay: '0.4s' }}>
            <Badge variant="secondary" className="gap-2 py-1.5">
              <Zap className="w-3.5 h-3.5 text-signal-orange" />
              Zero Dependencies
            </Badge>
            <Badge variant="secondary" className="gap-2 py-1.5">
              <Package className="w-3.5 h-3.5 text-dock-blue" />
              {'<10MB Binary'}
            </Badge>
            <Badge variant="secondary" className="gap-2 py-1.5">
              <Lock className="w-3.5 h-3.5 text-healthy" />
              Auto TLS
            </Badge>
          </div>
        </div>

        {/* Terminal Preview */}
        <div className="max-w-3xl mx-auto animate-fade-in-up" style={{ animationDelay: '0.5s' }}>
          <div className="terminal glow-blue">
            <div className="terminal-header">
              <div className="terminal-dot bg-[#FF5F56]" />
              <div className="terminal-dot bg-[#FFBD2E]" />
              <div className="terminal-dot bg-[#27C93F]" />
              <span className="ml-3 text-xs text-[#8B949E] font-mono">docker-compose.yml</span>
            </div>
            <div className="terminal-body text-[#E6EDF3]">
              <div><span className="syntax-key">version</span>: <span className="syntax-string">"3.8"</span></div>
              <div className="mt-2"><span className="syntax-key">services</span>:</div>
              <div className="ml-4"><span className="syntax-comment"># Your app - just add labels</span></div>
              <div className="ml-4"><span className="syntax-key">api</span>:</div>
              <div className="ml-8"><span className="syntax-key">image</span>: <span className="syntax-string">myapp/api:latest</span></div>
              <div className="ml-8"><span className="syntax-key">labels</span>:</div>
              <div className="ml-12"><span className="syntax-key">dr.enable</span>: <span className="syntax-string">"true"</span></div>
              <div className="ml-12"><span className="syntax-key">dr.host</span>: <span className="syntax-string">"api.example.com"</span></div>
              <div className="ml-12"><span className="syntax-key">dr.tls</span>: <span className="syntax-string">"auto"</span></div>
              <div className="mt-4 ml-4"><span className="syntax-comment"># That's it. HTTPS is live. </span><span className="syntax-value">🚀</span></div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
