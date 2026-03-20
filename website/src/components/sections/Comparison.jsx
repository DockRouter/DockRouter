import { Check, X, Ship } from 'lucide-react'
import { comparisonData } from '../../data/comparison'
import { useIntersection } from '../../hooks/use-intersection'
import { cn } from '../../lib/utils'

export function Comparison() {
  const [ref, isVisible] = useIntersection({ threshold: 0.1 })

  return (
    <section id="compare" className="py-24 section-alt">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="text-center mb-16">
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-bold text-[var(--text-primary)] mb-4">
            Why{' '}
            <span className="gradient-text-blue">DockRouter</span>?
          </h2>
          <p className="text-lg text-[var(--text-secondary)] max-w-2xl mx-auto">
            Traefik is great but complex. DockRouter is Traefik's simplicity promise, actually delivered.
          </p>
        </div>

        <div ref={ref} className={`reveal ${isVisible ? 'visible' : ''}`}>
          <div className="glass-card overflow-hidden p-0">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border-color)]">
                    {comparisonData.headers.map((header, i) => (
                      <th
                        key={header}
                        className={cn(
                          'px-6 py-4 text-left font-semibold whitespace-nowrap',
                          i === 0 ? 'text-[var(--text-primary)]' : 'text-center',
                          i === 1 && 'text-dock-blue bg-dock-blue/5'
                        )}
                      >
                        {i === 1 ? (
                          <div className="flex items-center justify-center gap-2">
                            <Ship className="w-4 h-4" />
                            {header}
                          </div>
                        ) : header}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {comparisonData.rows.map((row, index) => (
                    <tr
                      key={row.feature}
                      className={cn(
                        'border-b border-[var(--border-color)] last:border-0',
                        'hover:bg-[var(--bg-card-hover)] transition-colors'
                      )}
                    >
                      <td className="px-6 py-4 font-medium text-[var(--text-primary)] whitespace-nowrap">
                        {row.feature}
                      </td>
                      {['dockrouter', 'traefik', 'caddy', 'nginx'].map((product) => (
                        <td
                          key={product}
                          className={cn(
                            'px-6 py-4 text-center',
                            product === 'dockrouter' && 'bg-dock-blue/5'
                          )}
                        >
                          {row[product] ? (
                            <div className="flex justify-center">
                              <div className="w-6 h-6 rounded-full bg-healthy/10 flex items-center justify-center">
                                <Check className="w-4 h-4 text-healthy" />
                              </div>
                            </div>
                          ) : (
                            <div className="flex justify-center">
                              <div className="w-6 h-6 rounded-full bg-error/10 flex items-center justify-center">
                                <X className="w-4 h-4 text-error" />
                              </div>
                            </div>
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
