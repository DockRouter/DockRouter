import { createContext, useContext, useState } from 'react'
import { cn } from '../../lib/utils'

const TabsContext = createContext()

export function Tabs({ defaultValue, children, className }) {
  const [active, setActive] = useState(defaultValue)
  return (
    <TabsContext.Provider value={{ active, setActive }}>
      <div className={cn('', className)}>{children}</div>
    </TabsContext.Provider>
  )
}

export function TabsList({ children, className }) {
  return (
    <div className={cn(
      'inline-flex items-center gap-1 p-1 rounded-xl',
      'bg-[var(--bg-secondary)] border border-[var(--border-color)]',
      className
    )}>
      {children}
    </div>
  )
}

export function TabsTrigger({ value, children, className }) {
  const { active, setActive } = useContext(TabsContext)
  const isActive = active === value
  return (
    <button
      onClick={() => setActive(value)}
      className={cn(
        'px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200 cursor-pointer',
        isActive
          ? 'bg-dock-blue text-white shadow-sm'
          : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]',
        className
      )}
    >
      {children}
    </button>
  )
}

export function TabsContent({ value, children, className }) {
  const { active } = useContext(TabsContext)
  if (active !== value) return null
  return <div className={cn('mt-4 animate-fade-in', className)}>{children}</div>
}
