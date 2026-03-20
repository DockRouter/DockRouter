import { forwardRef } from 'react'
import { cn } from '../../lib/utils'

const variants = {
  default: 'bg-dock-blue text-white hover:bg-blue-600 shadow-lg shadow-dock-blue/20 hover:shadow-dock-blue/40',
  outline: 'border border-[var(--border-color)] text-[var(--text-primary)] hover:bg-[var(--bg-card)] hover:border-[var(--border-hover)]',
  ghost: 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-card)]',
  link: 'text-dock-blue hover:text-blue-400 underline-offset-4 hover:underline',
}

const sizes = {
  sm: 'h-8 px-3 text-sm rounded-lg',
  default: 'h-10 px-5 text-sm rounded-xl',
  lg: 'h-12 px-8 text-base rounded-xl',
  icon: 'h-10 w-10 rounded-xl',
}

export const Button = forwardRef(({ className, variant = 'default', size = 'default', ...props }, ref) => (
  <button
    ref={ref}
    className={cn(
      'inline-flex items-center justify-center gap-2 font-medium transition-all duration-200 cursor-pointer whitespace-nowrap',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-dock-blue/50 focus-visible:ring-offset-2',
      'disabled:pointer-events-none disabled:opacity-50',
      'active:scale-[0.97]',
      variants[variant],
      sizes[size],
      className
    )}
    {...props}
  />
))

Button.displayName = 'Button'
