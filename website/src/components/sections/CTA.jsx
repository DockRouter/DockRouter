import { ArrowRight, BookOpen } from 'lucide-react'
import { Button } from '../ui/button'
import { useIntersection } from '../../hooks/use-intersection'

export function CTA() {
  const [ref, isVisible] = useIntersection({ threshold: 0.2 })

  return (
    <section className="py-24 relative overflow-hidden">
      {/* Background */}
      <div className="absolute inset-0 bg-gradient-to-br from-dock-blue/5 via-[#7C3AED]/5 to-signal-orange/5" />
      <div className="hero-grid" style={{ opacity: 0.4 }} />
      <div className="cta-glow" />

      {/* Floating accent dots */}
      <div className="absolute top-20 left-[15%] w-2 h-2 rounded-full bg-dock-blue/30 animate-float" />
      <div className="absolute top-40 right-[20%] w-1.5 h-1.5 rounded-full bg-signal-orange/30 animate-float-slow" />
      <div className="absolute bottom-20 left-[30%] w-2.5 h-2.5 rounded-full bg-[#7C3AED]/20 animate-float-reverse" />
      <div className="absolute bottom-32 right-[25%] w-1.5 h-1.5 rounded-full bg-dock-blue/20 animate-float" style={{ animationDelay: '2s' }} />

      <div ref={ref} className={`relative max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 text-center z-10 reveal ${isVisible ? 'visible' : ''}`}>
        <h2 className="text-3xl sm:text-4xl lg:text-5xl font-bold text-[var(--text-primary)] mb-6">
          Ready to Simplify
          <br />
          <span className="gradient-text">Your Ingress?</span>
        </h2>
        <p className="text-lg text-[var(--text-secondary)] max-w-2xl mx-auto mb-10">
          Deploy DockRouter in under 60 seconds. Zero config files, automatic TLS,
          and your containers are live.
        </p>
        <div className="flex flex-col sm:flex-row items-center justify-center gap-4">
          <a href="#get-started">
            <Button size="lg" className="text-base px-8">
              Get Started Now
              <ArrowRight className="w-4 h-4" />
            </Button>
          </a>
          <a href="https://github.com/DockRouter/dockrouter/tree/main/docs" target="_blank" rel="noopener noreferrer">
            <Button variant="outline" size="lg" className="text-base px-8">
              <BookOpen className="w-5 h-5" />
              Read the Docs
            </Button>
          </a>
        </div>
      </div>
    </section>
  )
}
