export function generateChainName(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  const binary = String.fromCharCode(...bytes)
  const b64 = btoa(binary).replace(/[+/=]/g, '')
  return 'chain-' + b64.slice(0, 8)
}
