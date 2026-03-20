import { cn } from '../../lib/utils'

const variants = {
  default: 'bg-dock-blue/10 text-dock-blue border-dock-blue/20',
  secondary: 'bg-[var(--bg-secondary)] text-[var(--text-secondary)] border-[var(--border-color)]',
  orange: 'bg-signal-orange/10 text-signal-orange border-signal-orange/20',
  green: 'bg-healthy/10 text-healthy border-healthy/20',
  outline: 'border-[var(--border-color)] text-[var(--text-muted)]',
}

export function Badge({ className, variant = 'default', ...props }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 px-3 py-1 text-xs font-medium rounded-full border',
        variants[variant],
        className
      )}
      {...props}
    />
  )
}
