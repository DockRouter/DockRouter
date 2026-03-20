import { useState, useEffect } from 'react'
import { Sun, Moon, Menu, X, Github, Ship } from 'lucide-react'
import { useTheme } from '../../hooks/use-theme'
import { Button } from '../ui/button'
import { cn } from '../../lib/utils'

const navLinks = [
  { label: 'Features', href: '#features' },
  { label: 'How It Works', href: '#how-it-works' },
  { label: 'Compare', href: '#compare' },
  { label: 'Docs', href: '#get-started' },
]

export function Navbar() {
  const { theme, toggleTheme } = useTheme()
  const [scrolled, setScrolled] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  useEffect(() => {
    const handleScroll = () => setScrolled(window.scrollY > 20)
    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [])

  return (
    <header className={cn(
      'fixed top-0 left-0 right-0 z-50 transition-all duration-300',
      scrolled ? 'glass-nav shadow-lg shadow-black/5' : 'bg-transparent'
    )}>
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16">
          {/* Logo */}
          <a href="#" className="flex items-center gap-2 group">
            <div className="w-8 h-8 rounded-lg bg-dock-blue/10 flex items-center justify-center group-hover:bg-dock-blue/20 transition-colors">
              <Ship className="w-5 h-5 text-dock-blue" />
            </div>
            <span className="text-lg font-bold">
              <span className="text-dock-blue">Dock</span>
              <span className="text-[var(--text-primary)]">Router</span>
            </span>
          </a>

          {/* Desktop Nav */}
          <nav className="hidden md:flex items-center gap-1">
            {navLinks.map(link => (
              <a
                key={link.href}
                href={link.href}
                className="px-4 py-2 text-sm font-medium text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors rounded-lg hover:bg-[var(--bg-card)]"
              >
                {link.label}
              </a>
            ))}
          </nav>

          {/* Right Actions */}
          <div className="flex items-center gap-2">
            <a
              href="https://github.com/DockRouter/dockrouter"
              target="_blank"
              rel="noopener noreferrer"
              className="hidden sm:flex"
            >
              <Button variant="ghost" size="icon">
                <Github className="w-5 h-5" />
              </Button>
            </a>
            <Button variant="ghost" size="icon" onClick={toggleTheme}>
              {theme === 'dark' ? <Sun className="w-5 h-5" /> : <Moon className="w-5 h-5" />}
            </Button>
            <a href="#get-started" className="hidden sm:block">
              <Button size="sm">Get Started</Button>
            </a>
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => setMobileOpen(!mobileOpen)}
            >
              {mobileOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
            </Button>
          </div>
        </div>
      </div>

      {/* Mobile Menu */}
      {mobileOpen && (
        <div className="md:hidden glass-nav border-t border-[var(--border-color)] animate-fade-in-down">
          <nav className="max-w-7xl mx-auto px-4 py-4 flex flex-col gap-1">
            {navLinks.map(link => (
              <a
                key={link.href}
                href={link.href}
                onClick={() => setMobileOpen(false)}
                className="px-4 py-3 text-sm font-medium text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded-lg hover:bg-[var(--bg-card)] transition-colors"
              >
                {link.label}
              </a>
            ))}
            <a href="#get-started" onClick={() => setMobileOpen(false)} className="mt-2">
              <Button className="w-full">Get Started</Button>
            </a>
          </nav>
        </div>
      )}
    </header>
  )
}
