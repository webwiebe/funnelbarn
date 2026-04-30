import React from 'react'

interface SkeletonProps {
  height?: number | string
  width?: number | string
  borderRadius?: number
  style?: React.CSSProperties
}

export function Skeleton({ height = 20, width = '100%', borderRadius = 4, style }: SkeletonProps) {
  return (
    <div style={{
      height, width, borderRadius,
      background: 'linear-gradient(90deg, #1a1d27 25%, #2a2d3a 50%, #1a1d27 75%)',
      backgroundSize: '200% 100%',
      animation: 'skeleton-shimmer 1.5s infinite',
      ...style,
    }} />
  )
}

// Inject keyframes once — use a style tag appended to head
const KEYFRAMES = `@keyframes skeleton-shimmer { 0%{background-position:200% 0} 100%{background-position:-200% 0} }`
if (typeof document !== 'undefined' && !document.getElementById('skeleton-kf')) {
  const s = document.createElement('style')
  s.id = 'skeleton-kf'
  s.textContent = KEYFRAMES
  document.head.appendChild(s)
}
