// Hand-written, unlike the rest of this directory: what the generated runtime
// does at run time, which tsc cannot say. It throws on a failure, and Node
// exits non-zero, so it needs no test framework and no type packages.
import { Client } from "./client.js"

function assert(ok: boolean, what: string): void {
  if (!ok) throw new Error(what)
}

// A fetch that records what it was given and answers with a session.
function stub(body: unknown): [typeof fetch, () => Request] {
  let last: Request | undefined
  const f = async (url: string | URL | Request, init?: RequestInit) => {
    last = new Request(String(url), init)
    return new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    })
  }
  return [f as unknown as typeof fetch, () => {
    assert(last !== undefined, "nothing was sent")
    return last as Request
  }]
}

const session = {
  apiUrl: "https://example.com/jmap/api",
  accounts: {},
  primaryAccounts: {},
  capabilities: {},
  username: "someone@example.com",
  state: "s1",
}

// Headers reach the server, and auth has the last word over them.
{
  const [fetch, sent] = stub(session)
  const c = new Client("https://example.com/.well-known/jmap", {
    fetch,
    auth: "tok",
    headers: { "X-Tenant": "acme", Authorization: "overridden" },
  })
  await c.session()
  const h = sent().headers
  assert(h.get("X-Tenant") === "acme", "the extra header was not sent")
  assert(h.get("Authorization") === "Bearer tok", "auth did not win over headers")
  assert(h.get("Accept") === "application/json", "Accept was lost")
}

// A function auth sees the extra headers already set, and can read them.
{
  const [fetch, sent] = stub(session)
  const c = new Client("https://example.com/.well-known/jmap", {
    fetch,
    headers: { "X-Tenant": "acme" },
    auth: (headers) => {
      headers.set("Authorization", `Tenant ${headers.get("X-Tenant")}`)
    },
  })
  await c.session()
  assert(sent().headers.get("Authorization") === "Tenant acme", "auth ran before the headers")
}

// No headers option is the same request it was before.
{
  const [fetch, sent] = stub(session)
  const c = new Client("https://example.com/.well-known/jmap", { fetch, auth: "tok" })
  await c.session()
  assert(sent().headers.get("Authorization") === "Bearer tok", "auth alone stopped working")
}

// The session is cached, so a second call does not fetch again.
{
  let calls = 0
  const f = async () =>
    new Response(JSON.stringify(session), {
      headers: { "Content-Type": "application/json" },
    })
  const counted = (async (...args: unknown[]) => {
    calls++
    return (f as unknown as (...a: unknown[]) => Promise<globalThis.Response>)(...args)
  }) as unknown as typeof fetch
  const c = new Client("https://example.com/.well-known/jmap", { fetch: counted })
  await c.session()
  await c.session()
  assert(calls === 1, `the session was fetched ${calls} times, not once`)
}

console.log("ok")
