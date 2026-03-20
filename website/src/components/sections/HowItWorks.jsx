import { Tag, Search, Globe, ArrowRight } from 'lucide-react'
import { useIntersection } from '../../hooks/use-intersection'

const steps = [
  {
    icon: Tag,
    number: '01',
    title: 'Add Labels',
    description: 'Add Docker labels to your containers. Three lines is all it takes.',
    code: `dr.enable: "true"\ndr.host: "api.example.com"\ndr.tls: "auto"`,
    color: 'text-dock-blue bg-dock-blue/10',
  },
  {
    icon: Search,
    number: '02',
    title: 'DockRouter Discovers',
    description: 'DockRouter watches the Docker socket and detects your containers automatically.',
    code: `Container discovered\nHost: api.example.com\nTLS: provisioning...`,
    color: 'text-signal-orange bg-signal-orange/10',
  },
  {
    icon: Globe,
    number: '03',
    title: 'Traffic Routes',
    description: 'HTTPS is live. Traffic routes to your container with TLS, load balancing, and monitoring.',
    code: `https://api.example.com\nStatus: 200 OK\nTLS: valid (Let's Encrypt)`,
    color: 'text-healthy bg-healthy/10',
  },
]

export function HowItWorks() {
  const [ref, isVisible] = useIntersection({ threshold: 0.1 })

  return (
    <section id="how-it-works" className="py-24 section-alt">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-bold text-[var(--text-primary)] mb-4">
            Up and Running in{' '}
            <span className="gradient-text">60 Seconds</span>
          </h2>
          <p className="text-lg text-[var(--text-secondary)]">
            No config files. No complex setup. Just labels and go.
          </p>
        </div>

        {/* Steps */}
        <div ref={ref} className="grid grid-cols-1 lg:grid-cols-3 gap-6 lg:gap-8">
          {steps.map((step, index) => (
            <div
              key={step.number}
              className={`reveal ${isVisible ? 'visible' : ''} relative`}
              style={{ transitionDelay: `${index * 150}ms` }}
            >
              {/* Arrow between steps (desktop) */}
              {index < steps.length - 1 && (
                <div className="hidden lg:flex absolute top-1/2 -right-4 z-10 text-[var(--text-muted)]">
                  <ArrowRight className="w-6 h-6" />
                </div>
              )}

              <div className="glass-card h-full p-8">
                {/* Step number + icon */}
                <div className="flex items-center gap-4 mb-6">
                  <div className={`w-14 h-14 rounded-2xl flex items-center justify-center ${step.color}`}>
                    <step.icon className="w-7 h-7" />
                  </div>
                  <span className="text-4xl font-extrabold text-[var(--text-primary)] opacity-10">
                    {step.number}
                  </span>
                </div>

                {/* Content */}
                <h3 className="text-xl font-bold text-[var(--text-primary)] mb-2">
                  {step.title}
                </h3>
                <p className="text-sm text-[var(--text-muted)] mb-6 leading-relaxed">
                  {step.description}
                </p>

                {/* Code snippet */}
                <div className="bg-[#0D1117] rounded-lg p-4 font-mono text-xs leading-relaxed">
                  {step.code.split('\n').map((line, i) => (
                    <div key={i} className="text-[#E6EDF3]">
                      <span className="syntax-key">{line.split(':')[0]}</span>
                      {line.includes(':') && <span className="syntax-string">:{line.split(':').slice(1).join(':')}</span>}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
