// A rubric version in a rubric URL: positive, no leading zero, at most nine digits.
export function match(param: string): boolean {
  return /^[1-9][0-9]{0,8}$/.test(param);
}
