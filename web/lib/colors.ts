// Client colors follow the client's `slot` (first appearance in the whole
// log), so a client keeps its color across ranges. Slots 0 to 4 have a
// validated hue; the server sends slot -1 for the 6th client on, which folds
// into "Other" (neutral gray). The values
// live in globals.css as --series-* so dark and light swap with the theme.

export const MAX_SLOTS = 5;

export const isOther = (slot: number) => slot < 0 || slot >= MAX_SLOTS;

export function seriesVar(slot: number) {
  return isOther(slot) ? "var(--series-other)" : `var(--series-${slot})`;
}
