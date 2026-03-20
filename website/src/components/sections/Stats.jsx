import { FlaskConical, HardDrive, Blocks, GitFork } from 'lucide-react'
import { useIntersection } from '../../hooks/use-intersection'

const stats = [
  {
    icon: FlaskConical,
    value: '90%+',
    label: 'Test Coverage',
    color: 'text-healthy',
    bgColor: 'bg-healthy/10',
  },
  {
    icon: HardDrive,
    value: '<10MB',
    label: 'Binary Size',
    color: 'text-dock-blue',
    bgColor: 'bg-dock-blue/10',
  },
  {
    icon: Blocks,
    value: '0',
    label: 'Dependencies',
    color: 'text-signal-orange',
    bgColor: 'bg-signal-orange/10',
  },
  {
    icon: GitFork,
    value: '4',
    label: 'LB Strategies',
    color: 'text-[#8B5CF6]',
    bgColor: 'bg-[#8B5CF6]/10',
  },
]

export function Stats() {
  const [ref, isVisible] = useIntersection({ threshold: 0.2 })

  return (
    <section className="py-24">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div ref={ref} className="grid grid-cols-2 lg:grid-cols-4 gap-4 sm:gap-6">
          {stats.map((stat, index) => (
            <div
              key={stat.label}
              className={`reveal ${isVisible ? 'visible' : ''} glass-card p-8 text-center group`}
              style={{ transitionDelay: `${index * 100}ms` }}
            >
              <div className={`w-14 h-14 rounded-2xl ${stat.bgColor} flex items-center justify-center mx-auto mb-4 group-hover:scale-110 transition-transform`}>
                <stat.icon className={`w-7 h-7 ${stat.color}`} />
              </div>
              <div className={`text-4xl sm:text-5xl font-extrabold ${stat.color} mb-2`}>
                {stat.value}
              </div>
              <div className="text-sm text-[var(--text-muted)] font-medium">
                {stat.label}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
