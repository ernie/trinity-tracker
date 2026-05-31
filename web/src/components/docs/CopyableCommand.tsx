import { useState } from "react";

interface CopyableCommandProps {
  /**
   * The shell command to render and copy. Plain string only — pre/code
   * formatting is handled by the component, the caller just supplies
   * the text.
   */
  children: string;
}

// Block-level shell command with a copy-to-clipboard button. Mirrors
// the DocsH2 anchor and AccountPage token-copy patterns: writeText →
// flip a 'copied' flag → revert after 1500ms.
export function CopyableCommand({ children }: CopyableCommandProps) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    void navigator.clipboard.writeText(children).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => {
        /* older browsers without clipboard API — silent no-op */
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
