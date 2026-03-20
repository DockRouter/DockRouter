import { cn } from '../../lib/utils'

export function Card({ className, ...props }) {
  return (
    <div
      className={cn('glass-card p-6', className)}
      {...props}
    />
  )
}

export function CardHeader({ className, ...props }) {
  return <div className={cn('flex flex-col gap-2 mb-4', className)} {...props} />
}

export function CardTitle({ className, ...props }) {
  return <h3 className={cn('text-lg font-semibold text-[var(--text-primary)]', className)} {...props} />
}

export function CardDescription({ className, ...props }) {
  return <p className={cn('text-sm text-[var(--text-muted)] leading-relaxed', className)} {...props} />
}

export function CardContent({ className, ...props }) {
  return <div className={cn('', className)} {...props} />
}
