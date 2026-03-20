import { useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { useIntersection } from '../../hooks/use-intersection'

const codeBlocks = {
  compose: {
    filename: 'docker-compose.yml',
    raw: `version: "3.8"\n\nservices:\n  dockrouter:\n    image: dockrouter/dockrouter:latest\n    ports:\n      - "80:80"\n      - "443:443"\n      - "9090:9090"\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock:ro\n      - dockrouter-data:/data\n    environment:\n      - DR_ACME_EMAIL=you@example.com\n\n  api:\n    image: myapp/api:latest\n    labels:\n      dr.enable: "true"\n      dr.host: "api.example.com"\n      dr.tls: "auto"\n      dr.ratelimit: "100/m"\n\nvolumes:\n  dockrouter-data:`,
    lines: [
      { text: 'version: "3.8"', tokens: [{ type: 'key', text: 'version' }, { type: 'plain', text: ': ' }, { type: 'string', text: '"3.8"' }] },
      { text: '' },
      { text: 'services:', tokens: [{ type: 'key', text: 'services' }, { type: 'plain', text: ':' }] },
      { text: '  dockrouter:', tokens: [{ type: 'comment', text: '  ' }, { type: 'key', text: 'dockrouter' }, { type: 'plain', text: ':' }] },
      { text: '    image: dockrouter/dockrouter:latest', tokens: [{ type: 'plain', text: '    ' }, { type: 'key', text: 'image' }, { type: 'plain', text: ': ' }, { type: 'string', text: 'dockrouter/dockrouter:latest' }] },
      { text: '    ports:', tokens: [{ type: 'plain', text: '    ' }, { type: 'key', text: 'ports' }, { type: 'plain', text: ':' }] },
      { text: '      - "80:80"', tokens: [{ type: 'plain', text: '      - ' }, { type: 'string', text: '"80:80"' }] },
      { text: '      - "443:443"', tokens: [{ type: 'plain', text: '      - ' }, { type: 'string', text: '"443:443"' }] },
      { text: '      - "9090:9090"', tokens: [{ type: 'plain', text: '      - ' }, { type: 'string', text: '"9090:9090"' }] },
      { text: '    volumes:', tokens: [{ type: 'plain', text: '    ' }, { type: 'key', text: 'volumes' }, { type: 'plain', text: ':' }] },
      { text: '      - /var/run/docker.sock:/var/run/docker.sock:ro', tokens: [{ type: 'plain', text: '      - ' }, { type: 'string', text: '/var/run/docker.sock:/var/run/docker.sock:ro' }] },
      { text: '    environment:', tokens: [{ type: 'plain', text: '    ' }, { type: 'key', text: 'environment' }, { type: 'plain', text: ':' }] },
      { text: '      - DR_ACME_EMAIL=you@example.com', tokens: [{ type: 'plain', text: '      - ' }, { type: 'value', text: 'DR_ACME_EMAIL=you@example.com' }] },
      { text: '' },
      { text: '  # Your app - just add labels', tokens: [{ type: 'plain', text: '  ' }, { type: 'comment', text: '# Your app - just add labels' }] },
      { text: '  api:', tokens: [{ type: 'plain', text: '  ' }, { type: 'key', text: 'api' }, { type: 'plain', text: ':' }] },
      { text: '    image: myapp/api:latest', tokens: [{ type: 'plain', text: '    ' }, { type: 'key', text: 'image' }, { type: 'plain', text: ': ' }, { type: 'string', text: 'myapp/api:latest' }] },
      { text: '    labels:', tokens: [{ type: 'plain', text: '    ' }, { type: 'key', text: 'labels' }, { type: 'plain', text: ':' }] },
      { text: '      dr.enable: "true"', tokens: [{ type: 'plain', text: '      ' }, { type: 'key', text: 'dr.enable' }, { type: 'plain', text: ': ' }, { type: 'string', text: '"true"' }] },
      { text: '      dr.host: "api.example.com"', tokens: [{ type: 'plain', text: '      ' }, { type: 'key', text: 'dr.host' }, { type: 'plain', text: ': ' }, { type: 'string', text: '"api.example.com"' }] },
      { text: '      dr.tls: "auto"', tokens: [{ type: 'plain', text: '      ' }, { type: 'key', text: 'dr.tls' }, { type: 'plain', text: ': ' }, { type: 'string', text: '"auto"' }] },
      { text: '      dr.ratelimit: "100/m"', tokens: [{ type: 'plain', text: '      ' }, { type: 'key', text: 'dr.ratelimit' }, { type: 'plain', text: ': ' }, { type: 'string', text: '"100/m"' }] },
    ],
  },
  docker: {
    filename: 'terminal',
    raw: `docker run -d \\\n  --name dockrouter \\\n  -p 80:80 -p 443:443 -p 9090:9090 \\\n  -v /var/run/docker.sock:/var/run/docker.sock:ro \\\n  -v dockrouter-data:/data \\\n  -e DR_ACME_EMAIL=you@example.com \\\n  dockrouter/dockrouter:latest`,
    lines: [
      { tokens: [{ type: 'keyword', text: 'docker run' }, { type: 'flag', text: ' -d' }, { type: 'plain', text: ' \\' }] },
      { tokens: [{ type: 'plain', text: '  ' }, { type: 'flag', text: '--name' }, { type: 'plain', text: ' ' }, { type: 'string', text: 'dockrouter' }, { type: 'plain', text: ' \\' }] },
      { tokens: [{ type: 'plain', text: '  ' }, { type: 'flag', text: '-p' }, { type: 'plain', text: ' ' }, { type: 'value', text: '80:80' }, { type: 'plain', text: ' ' }, { type: 'flag', text: '-p' }, { type: 'plain', text: ' ' }, { type: 'value', text: '443:443' }, { type: 'plain', text: ' ' }, { type: 'flag', text: '-p' }, { type: 'plain', text: ' ' }, { type: 'value', text: '9090:9090' }, { type: 'plain', text: ' \\' }] },
      { tokens: [{ type: 'plain', text: '  ' }, { type: 'flag', text: '-v' }, { type: 'plain', text: ' ' }, { type: 'string', text: '/var/run/docker.sock:/var/run/docker.sock:ro' }, { type: 'plain', text: ' \\' }] },
      { tokens: [{ type: 'plain', text: '  ' }, { type: 'flag', text: '-v' }, { type: 'plain', text: ' ' }, { type: 'string', text: 'dockrouter-data:/data' }, { type: 'plain', text: ' \\' }] },
      { tokens: [{ type: 'plain', text: '  ' }, { type: 'flag', text: '-e' }, { type: 'plain', text: ' ' }, { type: 'value', text: 'DR_ACME_EMAIL=you@example.com' }, { type: 'plain', text: ' \\' }] },
      { tokens: [{ type: 'plain', text: '  ' }, { type: 'string', text: 'dockrouter/dockrouter:latest' }] },
    ],
  },
  install: {
    filename: 'terminal',
    raw: `curl -sL https://raw.githubusercontent.com/DockRouter/dockrouter/main/install.sh | bash`,
    lines: [
      { tokens: [{ type: 'keyword', text: 'curl' }, { type: 'flag', text: ' -sL' }, { type: 'plain', text: ' ' }, { type: 'url', text: 'https://raw.githubusercontent.com/DockRouter/dockrouter/main/install.sh' }, { type: 'plain', text: ' | ' }, { type: 'keyword', text: 'bash' }] },
    ],
  },
}

const tokenClass = {
  key: 'syntax-key',
  string: 'syntax-string',
  value: 'syntax-value',
  comment: 'syntax-comment',
  keyword: 'syntax-keyword',
  flag: 'syntax-flag',
  url: 'syntax-url',
  plain: '',
}

function CopyButton({ text }) {
  const [copied, setCopied] = useState(false)

  const copy = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <button
      onClick={copy}
      className="absolute top-3 right-3 p-2 rounded-lg bg-white/5 hover:bg-white/10 text-[#8B949E] hover:text-white transition-all cursor-pointer"
      title="Copy to clipboard"
    >
      {copied ? <Check className="w-4 h-4 text-healthy" /> : <Copy className="w-4 h-4" />}
    </button>
  )
}

function CodeBlock({ block }) {
  return (
    <div className="terminal relative">
      <div className="terminal-header">
        <div className="terminal-dot bg-[#FF5F56]" />
        <div className="terminal-dot bg-[#FFBD2E]" />
        <div className="terminal-dot bg-[#27C93F]" />
        <span className="ml-3 text-xs text-[#8B949E] font-mono">{block.filename}</span>
      </div>
      <CopyButton text={block.raw} />
      <div className="terminal-body text-[#E6EDF3]">
        {block.lines.map((line, i) => (
          <div key={i} className={line.tokens ? '' : 'h-4'}>
            {line.tokens?.map((token, j) => (
              <span key={j} className={tokenClass[token.type]}>
                {token.text}
              </span>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

export function CodeExample() {
  const [ref, isVisible] = useIntersection({ threshold: 0.1 })

  return (
    <section id="get-started" className="py-24">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="text-center mb-12">
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-bold text-[var(--text-primary)] mb-4">
            Get Started in{' '}
            <span className="gradient-text">Seconds</span>
          </h2>
          <p className="text-lg text-[var(--text-secondary)]">
            Choose your preferred installation method.
          </p>
        </div>

        <div ref={ref} className={`reveal ${isVisible ? 'visible' : ''}`}>
          <Tabs defaultValue="compose">
            <div className="flex justify-center mb-6">
              <TabsList>
                <TabsTrigger value="compose">Docker Compose</TabsTrigger>
                <TabsTrigger value="docker">Docker Run</TabsTrigger>
                <TabsTrigger value="install">Quick Install</TabsTrigger>
              </TabsList>
            </div>

            <TabsContent value="compose">
              <CodeBlock block={codeBlocks.compose} />
            </TabsContent>
            <TabsContent value="docker">
              <CodeBlock block={codeBlocks.docker} />
            </TabsContent>
            <TabsContent value="install">
              <CodeBlock block={codeBlocks.install} />
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </section>
  )
}
