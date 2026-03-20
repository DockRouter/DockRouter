import { Ship, Github, BookOpen, MessageCircle } from 'lucide-react'

const footerLinks = {
  Product: [
    { label: 'Features', href: '#features' },
    { label: 'How It Works', href: '#how-it-works' },
    { label: 'Comparison', href: '#compare' },
    { label: 'Get Started', href: '#get-started' },
  ],
  Resources: [
    { label: 'Documentation', href: 'https://github.com/DockRouter/dockrouter/tree/main/docs' },
    { label: 'Examples', href: 'https://github.com/DockRouter/dockrouter/tree/main/examples' },
    { label: 'Changelog', href: 'https://github.com/DockRouter/dockrouter/blob/main/CHANGELOG.md' },
    { label: 'Contributing', href: 'https://github.com/DockRouter/dockrouter/blob/main/CONTRIBUTING.md' },
  ],
  Connect: [
    { label: 'GitHub', href: 'https://github.com/DockRouter/dockrouter', icon: Github },
    { label: 'Issues', href: 'https://github.com/DockRouter/dockrouter/issues', icon: MessageCircle },
    { label: 'Docs', href: 'https://github.com/DockRouter/dockrouter/tree/main/docs', icon: BookOpen },
  ],
}

export function Footer() {
  return (
    <footer className="border-t border-[var(--border-color)] bg-[var(--bg-primary)]">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-8">
          {/* Brand */}
          <div className="col-span-2 md:col-span-1">
            <a href="#" className="flex items-center gap-2 mb-4">
              <div className="w-8 h-8 rounded-lg bg-dock-blue/10 flex items-center justify-center">
                <Ship className="w-5 h-5 text-dock-blue" />
              </div>
              <span className="text-lg font-bold">
                <span className="text-dock-blue">Dock</span>
                <span className="text-[var(--text-primary)]">Router</span>
              </span>
            </a>
            <p className="text-sm text-[var(--text-muted)] leading-relaxed">
              Zero-dependency Docker-native ingress router with automatic TLS.
            </p>
          </div>

          {/* Link Columns */}
          {Object.entries(footerLinks).map(([title, links]) => (
            <div key={title}>
              <h4 className="text-sm font-semibold text-[var(--text-primary)] mb-4">{title}</h4>
              <ul className="space-y-3">
                {links.map(link => (
                  <li key={link.label}>
                    <a
                      href={link.href}
                      target={link.href.startsWith('http') ? '_blank' : undefined}
                      rel={link.href.startsWith('http') ? 'noopener noreferrer' : undefined}
                      className="text-sm text-[var(--text-muted)] hover:text-dock-blue transition-colors flex items-center gap-2"
                    >
                      {link.icon && <link.icon className="w-4 h-4" />}
                      {link.label}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {/* Bottom Bar */}
        <div className="mt-12 pt-8 border-t border-[var(--border-color)] flex flex-col sm:flex-row justify-between items-center gap-4">
          <p className="text-sm text-[var(--text-muted)]">
            &copy; {new Date().getFullYear()} DockRouter. MIT License.
          </p>
          <p className="text-sm text-[var(--text-muted)]">
            Built with Go. Zero dependencies.
          </p>
        </div>
      </div>
    </footer>
  )
}
