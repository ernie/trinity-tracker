import { useState } from "react";
import { useMySources } from "../hooks/useMySource";
import { MyServersDrawer } from "./MyServersDrawer";

export function MySourceButton() {
  const { data, refresh } = useMySources();
  const [open, setOpen] = useState(false);

  const actives = data.sources.filter((s) => s.status === "active");

  let label = "Add servers";
  let cls = "my-source-btn";
  if (data.has_pending) {
    label = "Request pending";
    cls += " my-source-btn-pending";
  } else if (actives.length > 0) {
    label =
      actives.length === 1 ? "My servers" : `My servers (${actives.length})`;
    cls += " my-source-btn-active";
  }

  return (
    <>
      <button className={cls} onClick={() => setOpen(true)}>
        {label}
      </button>
      {open && (
        <MyServersDrawer
          sources={data.sources}
          hasPending={data.has_pending}
          onClose={() => setOpen(false)}
          onRefresh={refresh}
        />
      )}
    </>
  );
}
