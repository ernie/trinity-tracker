import { useMemo } from 'react'

// Q3 color codes: ^0-^9
const Q3_COLORS: Record<string, string> = {
  '0': '#000000', // black
  '1': '#ff0000', // red
  '2': '#00ff00', // green
  '3': '#ffff00', // yellow
  '4': '#0000ff', // blue
  '5': '#00ffff', // cyan
  '6': '#ff00ff', // magenta
  '7': '#ffffff', // white
  '8': '#ff8800', // orange
  '9': '#888888', // gray
}

interface ColoredSegment {
  text: string
  color: string
}

function parseQ3ColorCodes(name: string): ColoredSegment[] {
  const segments: ColoredSegment[] = []
  let currentColor = '#ffffff' // default white
  let currentText = ''
  let i = 0

  while (i < name.length) {
    if (name[i] === '^' && i + 1 < name.length) {
      const colorCode = name[i + 1]
      if (Q3_COLORS[colorCode]) {
        // Save current segment if it has text
        if (currentText) {
          segments.push({ text: currentText, color: currentColor })
          currentText = ''
        }
        currentColor = Q3_COLORS[colorCode]
        i += 2
        continue
      }
    }
    currentText += name[i]
    i++
  }

  // Add remaining text
  if (currentText) {
    segments.push({ text: currentText, color: currentColor })
  }

  return segments
}

interface ColoredTextProps {
  text: string
  className?: string
}

export function ColoredText({ text, className }: ColoredTextProps) {
  const segments = useMemo(() => parseQ3ColorCodes(text), [text])

  return (
    <span className={className}>
      {segments.map((segment, i) => (
        <span key={i} style={{ color: segment.color }}>
          {segment.text}
        </span>
      ))}
    </span>
  )
}
