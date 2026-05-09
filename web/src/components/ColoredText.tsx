import { useMemo } from 'react'

// Q3 chat color codes ^0–^7 (stock palette). Engine forks may extend
// the table (e.g., ^8/^9 = orange/gray in some builds); we render those
// as literal text since stock Q3 doesn't support them.
const Q3_COLORS: Record<string, string> = {
  '0': 'var(--q3-color-0)',
  '1': 'var(--q3-color-1)',
  '2': 'var(--q3-color-2)',
  '3': 'var(--q3-color-3)',
  '4': 'var(--q3-color-4)',
  '5': 'var(--q3-color-5)',
  '6': 'var(--q3-color-6)',
  '7': 'var(--q3-color-7)',
}

const DEFAULT_COLOR = 'var(--q3-color-7)' // white

interface ColoredSegment {
  text: string
  color: string
}

function parseQ3ColorCodes(name: string): ColoredSegment[] {
  const segments: ColoredSegment[] = []
  let currentColor = DEFAULT_COLOR
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
