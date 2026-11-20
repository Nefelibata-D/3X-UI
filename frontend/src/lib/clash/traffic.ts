export function displayTrafficBytes(bytes: number, serverView: boolean): number {
  return serverView ? 2 * bytes : bytes;
}
