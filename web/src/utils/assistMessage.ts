export function assistMessage(assistType: string | undefined): string {
  switch (assistType) {
    case "frag":
    case "return":
    case "carry":
      return "assisted a capture!";
    case "damage":
      return "assisted a frag!";
    case "skull":
      return "assisted a skull delivery!";
    case "obelisk":
      return "helped destroy the obelisk!";
    default:
      return "earned an assist!";
  }
}
