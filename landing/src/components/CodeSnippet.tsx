import { useState } from 'react';

const dockerLines = [
  { num: 1, type: 'comment', text: '// Clone and start LeadEcho' },
  { num: 2, type: 'cmd', text: 'git clone https://github.com/rohansx/leadecho' },
  { num: 3, type: 'empty', text: '' },
  { num: 4, type: 'comment', text: '// Configure your API keys' },
  { num: 5, type: 'cmd', text: 'cp .env.example .env' },
  { num: 6, type: 'empty', text: '' },
  { num: 7, type: 'comment', text: '// Start all services' },
  { num: 8, type: 'cmd', text: 'docker compose up -d' },
  { num: 9, type: 'empty', text: '' },
  { num: 10, type: 'comment', text: '// Dashboard at http://localhost:5173' },
  { num: 11, type: 'comment', text: '// API at http://localhost:8080' },
];

const manualLines = [
  { num: 1, type: 'comment', text: '// Install dependencies' },
  { num: 2, type: 'cmd', text: 'cd backend && go build ./...' },
  { num: 3, type: 'cmd', text: 'cd dashboard && pnpm install' },
  { num: 4, type: 'empty', text: '' },
  { num: 5, type: 'comment', text: '// Start Postgres, then run migrations' },
  { num: 6, type: 'cmd', text: 'make migrate-up' },
  { num: 7, type: 'empty', text: '' },
  { num: 8, type: 'comment', text: '// Start backend + frontend' },
  { num: 9, type: 'cmd', text: 'go run ./cmd/server &' },
  { num: 10, type: 'cmd', text: 'pnpm dev' },
];

export default function CodeSnippet() {
  const [activeTab, setActiveTab] = useState<'docker' | 'manual'>('docker');
  const lines = activeTab === 'docker' ? dockerLines : manualLines;

  return (
    <div className="code-snippet">
      <div className="code-tabs">
        <button
          className={`code-tab${activeTab === 'docker' ? ' active' : ''}`}
          onClick={() => setActiveTab('docker')}
        >
          Docker
        </button>
        <button
          className={`code-tab${activeTab === 'manual' ? ' active' : ''}`}
          onClick={() => setActiveTab('manual')}
        >
          Manual
        </button>
      </div>
      <div className="code-body">
        {lines.map((line) => (
          <div key={line.num}>
            <span className="code-line-num">{line.num}</span>
            {line.type === 'comment' && <span className="code-comment">{line.text}</span>}
            {line.type === 'cmd' && <span className="code-cmd">{line.text}</span>}
            {line.type === 'empty' && null}
          </div>
        ))}
      </div>
    </div>
  );
}
