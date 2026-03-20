import { ThemeProvider } from './hooks/use-theme'
import { Navbar } from './components/layout/Navbar'
import { Footer } from './components/layout/Footer'
import { Hero } from './components/sections/Hero'
import { TrustBar } from './components/sections/TrustBar'
import { Features } from './components/sections/Features'
import { HowItWorks } from './components/sections/HowItWorks'
import { CodeExample } from './components/sections/CodeExample'
import { Comparison } from './components/sections/Comparison'
import { Stats } from './components/sections/Stats'
import { CTA } from './components/sections/CTA'

export default function App() {
  return (
    <ThemeProvider>
      <div className="min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)] transition-colors duration-300">
        <Navbar />
        <main>
          <Hero />
          <TrustBar />
          <Features />
          <HowItWorks />
          <CodeExample />
          <Comparison />
          <Stats />
          <CTA />
        </main>
        <Footer />
      </div>
    </ThemeProvider>
  )
}
