import { Container, ShieldCheck, BarChart3, PieChart } from 'lucide-react'
import { useIntersection } from '../../hooks/use-intersection'

const tools = [
  { name: 'Docker', icon: Container, color: 'text-[#2496ED]' },
  { name: "Let's Encrypt", icon: ShieldCheck, color: 'text-[#003A70]' },
  { name: 'Prometheus', icon: BarChart3, color: 'text-[#E6522C]' },
  { name: 'Grafana', icon: PieChart, color: 'text-[#F46800]' },
]

export function TrustBar() {
  const [ref, isVisible] = useIntersection({ threshold: 0.3 })

  return (
    <section ref={ref} className="py-16 border-y border-[var(--border-color)]">
      <div className={`max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 reveal ${isVisible ? 'visible' : ''}`}>
        <p className="text-center text-sm font-medium text-[var(--text-muted)] mb-8 uppercase tracking-widest">
          Works seamlessly with
        </p>
        <div className="flex flex-wrap items-center justify-center gap-8 sm:gap-16">
          {tools.map(tool => (
            <div
              key={tool.name}
              className="flex items-center gap-3 text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors duration-300 group"
            >
              <tool.icon className={`w-6 h-6 opacity-50 group-hover:opacity-100 transition-opacity ${tool.color}`} />
              <span className="text-sm font-medium">{tool.name}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
