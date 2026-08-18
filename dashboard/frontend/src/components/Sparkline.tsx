import React from 'react'

interface SparklineProps {
  data: number[]
  color?: string
  fillColor?: string
  height?: number
  width?: number
  min?: number
  max?: number
}

export const Sparkline: React.FC<SparklineProps> = ({
  data,
  color = '#10b981',
  fillColor = 'rgba(16, 185, 129, 0.1)',
  height = 50,
  width = 240,
  min: customMin,
  max: customMax,
}) => {
  if (!data || data.length < 2) {
    return <div style={{ height }} className="w-full flex items-center justify-center text-[10px] text-zinc-500 font-mono">Collecting samples...</div>
  }

  const min = customMin !== undefined ? customMin : Math.min(...data)
  const max = customMax !== undefined ? customMax : Math.max(...data, min + 1)
  const range = max - min || 1

  const points = data.map((val, i) => {
    const x = (i / (data.length - 1)) * width
    const y = height - ((val - min) / range) * (height - 8) - 4
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })

  const pathD = `M ${points.join(' L ')}`
  const fillD = `M 0,${height} L ${points.join(' L ')} L ${width},${height} Z`

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full overflow-visible">
      {fillColor && <path d={fillD} fill={fillColor} />}
      <path
        d={pathD}
        fill="none"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
