// Mounts the <video> only while it's within about half a viewport of
// the screen: several autoplaying videos on one page can crash mobile
// Safari's renderer. Scrolling back starts it from frame 0, which suits
// short loops.
import { useEffect, useRef, useState } from "react";
import { PlayIcon } from "../PlayIcon";

interface LazyVideoProps {
  /** Tried before the MP4. */
  webm?: string;
  mp4: string;
  /** The clip's pixel size, which shapes the box before it loads. */
  width: number;
  height: number;
  /** Placeholder height in px until the video has been measured. */
  minHeight?: number;
  className?: string;
}

export function LazyVideo({
  webm,
  mp4,
  width,
  height,
  minHeight = 240,
  className,
}: LazyVideoProps) {
  const [visible, setVisible] = useState(false);
  const [blocked, setBlocked] = useState(false);
  const [measuredHeight, setMeasuredHeight] = useState<number | undefined>(
    undefined,
  );
  const ref = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
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
        if (!entry.isIntersecting) setBlocked(false);
        setVisible(entry.isIntersecting);
      },
      { rootMargin: "50% 0px" },
    );
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!visible) return;
    videoRef.current?.play().catch((err: unknown) => {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setBlocked(true);
      }
    });
  }, [visible]);

  const onPlay = () => {
    videoRef.current?.play().then(
      () => setBlocked(false),
      () => setBlocked(true),
    );
  };

  const placeholderHeight = measuredHeight ?? minHeight;

  return (
    <div
      ref={ref}
      className={className ? `lazy-video ${className}` : "lazy-video"}
      style={!visible ? { minHeight: `${placeholderHeight}px` } : undefined}
    >
      {visible && (
        <video
          ref={videoRef}
          width={width}
          height={height}
          autoPlay
          loop
          muted
          playsInline
        >
          {webm && <source src={webm} type="video/webm" />}
          <source src={mp4} type="video/mp4" />
        </video>
      )}
      {visible && blocked && (
        <button
          type="button"
          onClick={onPlay}
          className="lazy-video__play"
          aria-label="Play video"
        >
          <PlayIcon size={24} />
        </button>
      )}
    </div>
  );
}
