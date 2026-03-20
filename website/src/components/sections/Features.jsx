import {
  Zap, Lock, Tag, Package, RefreshCw, LayoutDashboard,
  Cable, BarChart3, Scale, Shield, CircuitBoard, ShieldCheck
} from 'lucide-react'
import { features } from '../../data/features'
import { Badge } from '../ui/badge'
import { useIntersection } from '../../hooks/use-intersection'
import { cn } from '../../lib/utils'

const iconMap = {
  Zap, Lock, Tag, Package, RefreshCw, LayoutDashboard,
  Cable, BarChart3, Scale, Shield, CircuitBoard, ShieldCheck,
}

export function Features() {
  const [ref, isVisible] = useIntersection({ threshold: 0.05 })

  return (
    <section id="features" className="py-24 relative">
      <div className="absolute inset-0 dot-pattern opacity-30" />
      <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <Badge className="mb-4">Features</Badge>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-bold text-[var(--text-primary)] mb-4">
            Built for Developers
            <br />
            <span className="gradient-text-blue">Who Ship</span>
          </h2>
          <p className="text-lg text-[var(--text-secondary)]">
            Everything you need for production Docker ingress. Nothing you don't.
          </p>
        </div>

        {/* Bento Grid */}
        <div ref={ref} className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {features.map((feature, index) => {
            const Icon = iconMap[feature.icon]
            return (
              <div
                key={feature.title}
                className={cn(
                  `reveal ${isVisible ? 'visible' : ''}`,
                  feature.span === 2 && 'sm:col-span-2 lg:col-span-2',
                  feature.highlight && 'gradient-border'
                )}
                style={{ transitionDelay: `${index * 60}ms` }}
              >
                <div className={cn(
                  'glass-card h-full p-6 sm:p-8',
                  feature.highlight && 'gradient-border-inner'
                )}>
                  <div className="flex items-start gap-4">
                    <div className={cn(
                      'w-12 h-12 rounded-xl flex items-center justify-center shrink-0',
                      feature.highlight
                        ? 'bg-dock-blue/10 text-dock-blue'
                        : 'bg-[var(--bg-secondary)] text-[var(--text-muted)]'
                    )}>
                      {Icon && <Icon className="w-6 h-6" />}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-2">
                        <h3 className="text-lg font-semibold text-[var(--text-primary)]">
                          {feature.title}
                        </h3>
                        {feature.badge && (
                          <Badge variant={feature.highlight ? 'default' : 'outline'} className="text-[10px] px-2 py-0.5">
                            {feature.badge}
                          </Badge>
                        )}
                      </div>
                      <p className="text-sm text-[var(--text-muted)] leading-relaxed">
                        {feature.description}
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}
