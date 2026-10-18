/** Returns `one` when n === 1, else `many`. */
export function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}
