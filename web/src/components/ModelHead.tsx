import { useEffect, useRef, useState } from "react";
import { PlayerPortrait } from "./PlayerPortrait";
import { parseModel } from "../utils/model";
import "./ModelHead.css";

interface ModelHeadProps {
  model?: string;
  team?: number;
  size?: "sm" | "md" | "lg" | "xl";
  className?: string;
}

// Canvas backing-store resolution; CSS scales it to the portrait box.
const RENDER_PX = 256;

export function ModelHead({
  model,
  team,
  size = "lg",
  className = "",
}: ModelHeadProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [live, setLive] = useState(false);

  useEffect(() => {
    if (!model) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    canvas.width = RENDER_PX;
    canvas.height = RENDER_PX;

    let released = false;
    let handle: { release: () => void } | null = null;
    // `live` is already false here (initial state, or reset by this effect's
    // own cleanup on a deps change) — don't set state synchronously in an
    // effect body or react-hooks/set-state-in-effect fires.

    (async () => {
      const { getSharedHeadRenderer } =
        await import("../utils/sharedHeadRenderer");
      if (released) return;
      const { modelName, skin } = parseModel(model);
      const teamSkin = team === 1 ? "red" : team === 2 ? "blue" : skin;
      handle = getSharedHeadRenderer().register(
        canvas,
        modelName,
        teamSkin,
        () => {
          if (!released) setLive(true);
        },
      );
    })();

    return () => {
      released = true;
      handle?.release();
      setLive(false);
    };
  }, [model, team]);

  return (
    <span className={`model-head ${live ? "is-live" : ""} ${className}`}>
      <PlayerPortrait model={model} team={team} size={size} />
      {model ? (
        <canvas
          key={`${model}-${team ?? ""}`}
          ref={canvasRef}
          aria-hidden="true"
        />
      ) : null}
    </span>
  );
}
