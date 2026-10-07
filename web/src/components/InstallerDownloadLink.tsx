import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import type { InstallerDownload } from "../hooks/useInstallerDownload";

interface InstallerDownloadLinkProps {
  download: InstallerDownload;
  className?: string;
  title?: string;
  children: ReactNode;
}

// A direct installer download is a plain link to GitHub; the install
// guide fallback stays inside the app's router.
export function InstallerDownloadLink({
  download,
  className,
  title,
  children,
}: InstallerDownloadLinkProps) {
  if (download.direct) {
    return (
      <a
        href={download.href}
        className={className}
        title={title}
        aria-label={title}
      >
        {children}
      </a>
    );
  }
  return (
    <Link
      to={download.href}
      className={className}
      title={title}
      aria-label={title}
    >
      {children}
    </Link>
  );
}
