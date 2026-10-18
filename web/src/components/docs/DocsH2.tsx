import { useState, type ReactNode } from "react";
import { CheckIcon } from "../CheckIcon";
import { PlatformChip } from "./PlatformChip";
import type { Platform } from "./platformStorage";

interface DocsH2Props {
  id: string;
  children: ReactNode;
  // Marks a whole section as platform-specific; PlatformNote suits a
  // single aside.
  platforms?: Platform | Platform[];
}

// The anchor next to the heading copies a link to this section.
export function DocsH2({ id, children, platforms }: DocsH2Props) {
  const [copied, setCopied] = useState(false);

  const copyAnchor = () => {
    const url = `${window.location.origin}${window.location.pathname}#${id}`;
    void navigator.clipboard.writeText(url).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => {
        /* clipboard unavailable */
      },
    );
  };

  return (
    <h2 id={id} className="docs-h2">
      <span className="docs-h2__text">{children}</span>
      {platforms && <PlatformChip platform={platforms} />}
      <button
        type="button"
        className="docs-h2__anchor"
        title={copied ? "Copied!" : "Copy link to this section"}
        aria-label={`Copy link to ${typeof children === "string" ? children : "this section"}`}
        onClick={copyAnchor}
      >
        {copied ? <CheckIcon /> : "#"}
      </button>
    </h2>
  );
}
