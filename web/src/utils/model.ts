// Parse a Q3A model string into model name + skin.
// "sarge" -> {sarge, default}; "sarge/krusade" -> {sarge, krusade};
// "*james" (Team Arena head) -> {james, default}.
export function parseModel(model: string): { modelName: string; skin: string } {
  const clean = model.startsWith("*") ? model.slice(1) : model;
  const parts = clean.split("/");
  return {
    modelName: parts[0].toLowerCase(),
    skin: (parts[1] || "default").toLowerCase(),
  };
}
