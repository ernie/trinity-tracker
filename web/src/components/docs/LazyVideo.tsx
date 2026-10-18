// Mounts the <video> only while it's within about half a viewport of
// the screen: several autoplaying videos on one page can crash mobile
// Safari's renderer. Scrolling back starts it from frame 0, which suits
// short loops.
import { useEffect, useRef, useState } from "react";

interface LazyVideoProps {
  /** Tried before the MP4. */
  webm?: string;
  mp4: string;
  /** Placeholder height in px until the video has been measured. */
  minHeight?: number;
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
  // Lets the observer callback read the latest value without
  // resubscribing.
  const visibleRef = useRef(visible);
  useEffect(() => {
    visibleRef.current = visible;
  }, [visible]);

  useEffect(() => {
    if (!ref.current) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        // Measured just before unmounting so the placeholder keeps the
        // page from jumping.
        if (!entry.isIntersecting && visibleRef.current && ref.current) {
          setMeasuredHeight(ref.current.offsetHeight);
        }
        setVisible(entry.isIntersecting);
      },
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
