// Cloudflare Pages Function. Forwards /api/* (HTTP and the /api/ws WebSocket)
// to the Go server named by BACKEND_URL, so the browser keeps talking to a
// single origin: the session cookie stays SameSite=Strict and no CORS is needed.
// Locally the Vite dev proxy does the same job (vite.config.ts).
//
// PROXY_SECRET is shared with the reverse proxy in front of the Go server
// (deploy/Caddyfile), which refuses anything that does not carry it and takes
// the player's address from X-Client-IP.
export async function onRequest({ request, env }) {
  if (!env.BACKEND_URL) return fail('Backend not configured.')

  const from = new URL(request.url)
  const out = new Request(new URL(from.pathname + from.search, env.BACKEND_URL), request)

  out.headers.delete('X-Client-IP')
  out.headers.delete('X-Proxy-Secret')
  const ip = request.headers.get('CF-Connecting-IP')
  if (ip) out.headers.set('X-Client-IP', ip)
  if (env.PROXY_SECRET) out.headers.set('X-Proxy-Secret', env.PROXY_SECRET)

  try {
    return await fetch(out)
  } catch {
    return fail('Backend unreachable.')
  }
}

function fail(error) {
  return new Response(JSON.stringify({ error }), {
    status: 502,
    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' },
  })
}
