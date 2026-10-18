// A rubric id in a rubric URL.
export function match(param: string): boolean {
  return /^[A-Za-z0-9_.-]{1,64}$/.test(param);
}
