import { useState } from 'react'
import { Copy, Check } from 'lucide-react'

interface CopyButtonProps {
  value: string
}

export function CopyButton({ value }: CopyButtonProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <button
      onClick={handleCopy}
      title="Copy to clipboard"
      style={{
        background: copied ? 'rgba(16,185,129,0.1)' : '#0f1117',
        border: `1px solid ${copied ? 'rgba(16,185,129,0.4)' : '#2a2d3a'}`,
        borderRadius: 7,
        color: copied ? '#10b981' : '#94a3b8',
        padding: '0.4rem 0.75rem',
        cursor: 'pointer',
        display: 'flex',
        alignItems: 'center',
        gap: 5,
        fontSize: 13,
        transition: 'all 0.2s',
      }}
    >
      {copied ? <Check size={13} /> : <Copy size={13} />}
      {copied ? 'Copied!' : 'Copy'}
    </button>
  )
}
