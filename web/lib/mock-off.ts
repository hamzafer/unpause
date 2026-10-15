// Production stand-in for lib/mock.ts (see next.config.ts): keeps the
// fixtures out of the static export.
export async function mockFetch(path: string): Promise<unknown> {
  throw new Error(`mock is off: ${path}`);
}
