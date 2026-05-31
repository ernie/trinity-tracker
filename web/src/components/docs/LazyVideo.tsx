// LazyVideo — mounts a <video> element only while its placeholder is
// within ~half a viewport of being visible. Without this, every
// autoplay video in a long docs page decodes frames continuously even
// when scrolled far away — multiple such videos plus other heavyweight
// page content can overwhelm mobile Safari's renderer process and
// trigger a tab reload.
//
// Mount/unmount is symmetric: scrolling away tears the <video> down
// (freeing decoder + frame buffer memory); scrolling back mounts a
// fresh element that auto-plays from frame 0. Acceptable for short
// looping effect demos; would be jarring for long-form video where
// playback position matters.
//
// Layout: the wrapper captures the video's rendered height the first
// time it's measured, then uses that as the placeholder height after
// unmounting. Before the first mount, falls back to `minHeight`
// (caller-supplied or a coarse default) — best we can do without
// knowing the video's intrinsic aspect ratio up front.
import { useEffect, useRef, useState } from "react";

interface LazyVideoProps {
  /** Optional WebM source — typically the smaller/preferred encoding. */
  webm?: string;
  /** MP4 source — required as the cross-browser fallback. */
  mp4: string;
  /** Pre-measurement placeholder min-height, in px. */
  minHeight?: number;
  /** Extra class on the wrapper div (rare; usually inherits parent layout). */
  className?: string;
}

export function LazyVideo({
  webm,
  mp4,
  minHeight = 240,
  className,
}: LazyVideoProps) {
  const [visible, setVisible] = useState(false);
  const [measuredHeight, setMeasuredHeight] = useState<number | undefined>(
    undefined,
  );
  const ref = useRef<HTMLDivElement>(null);
  // Mirror `visible` in a ref so the IntersectionObserver callback can
  // read the latest value without re-subscribing every toggle.
  const visibleRef = useRef(visible);
  useEffect(() => {
    visibleRef.current = visible;
  }, [visible]);

  useEffect(() => {
    if (!ref.current) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        // Measure the rendered video JUST before tearing it down so
        // the placeholder reserves the same vertical space and the
        // page doesn't jump.
        if (!entry.isIntersecting && visibleRef.current && ref.current) {
          setMeasuredHeight(ref.current.offsetHeight);
        }
        setVisible(entry.isIntersecting);
      },
      // Mount ~half a viewport ahead of the scroll edge so the video
      // is ready by the time the user reaches it.
      { rootMargin: "50% 0px" },
    );
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);

  const placeholderHeight = measuredHeight ?? minHeight;

  return (
    <div
      ref={ref}
      className={className}
      style={!visible ? { minHeight: `${placeholderHeight}px` } : undefined}
    >
      {visible && (
        <video autoPlay loop muted playsInline>
          {webm && <source src={webm} type="video/webm" />}
          <source src={mp4} type="video/mp4" />
        </video>
      )}
    </div>
  );
}
