import { useEffect, useRef, useState, type MouseEvent } from "react";
import { PlayerPortrait } from "./PlayerPortrait";
import { parseModel } from "../utils/model";
import type { HeadHandle } from "../utils/sharedHeadRenderer";
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
  const overlayRef = useRef<HTMLCanvasElement>(null);
  const handleRef = useRef<HeadHandle | null>(null);
  const pointerRef = useRef("mouse");
  const [live, setLive] = useState(false);

  useEffect(() => {
    if (!model) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    canvas.width = RENDER_PX;
    canvas.height = RENDER_PX;

    let released = false;
    let handle: HeadHandle | null = null;
    // `live` is already false here: the initial state, or reset by this
    // effect's cleanup.

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
        overlayRef.current,
      );
      handleRef.current = handle;
    })();

    return () => {
      released = true;
      handle?.release();
      handleRef.current = null;
      setLive(false);
    };
  }, [model, team]);

  // A tap on a head inside a link or button belongs to that control.
  const onClick = (e: MouseEvent<HTMLSpanElement>) => {
    if (
      pointerRef.current === "mouse" ||
      !e.currentTarget.closest('a, button, [role="button"], [role="link"]')
    ) {
      handleRef.current?.honk();
    }
  };

  return (
    <span
      className={`model-head ${live ? "is-live" : ""} ${className}`}
      onPointerDown={(e) => {
        pointerRef.current = e.pointerType;
      }}
      onPointerEnter={(e) => {
        if (e.pointerType === "mouse") handleRef.current?.honk();
      }}
      onClick={onClick}
    >
      <PlayerPortrait model={model} team={team} size={size} />
      {model ? (
        <>
          <canvas
            key={`${model}-${team ?? ""}`}
            ref={canvasRef}
            aria-hidden="true"
          />
          <canvas
            key={`honk-${model}-${team ?? ""}`}
            ref={overlayRef}
            className="model-head__honk"
            aria-hidden="true"
          />
        </>
      ) : null}
    </span>
  );
}
