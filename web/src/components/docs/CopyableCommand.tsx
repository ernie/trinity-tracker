import { useState } from "react";

interface CopyableCommandProps {
  children: string;
}

export function CopyableCommand({ children }: CopyableCommandProps) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    void navigator.clipboard.writeText(children).then(
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
    <div className="docs-copyable-command">
      <pre>
        <code>{children}</code>
      </pre>
      <button
        type="button"
        className="docs-copyable-command__copy"
        title={copied ? "Copied!" : "Copy command"}
        aria-label="Copy command to clipboard"
        onClick={copy}
      >
        {copied ? "Copied!" : "Copy"}
      </button>
    </div>
  );
}
