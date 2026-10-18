// A board or run id in a run URL.
export function match(param: string): boolean {
  return /^[A-Za-z0-9_-]{1,128}$/.test(param);
}
