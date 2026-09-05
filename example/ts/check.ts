// Hand-written, unlike the rest of this directory: what the generated runtime
// does at run time, which tsc cannot say. It throws on a failure, and Node
// exits non-zero, so it needs no test framework and no type packages.
import { Client, MethodErrors, SetErrors } from "./client.js"
import { fileIntoNewMailbox, type FileIntoNewMailboxResult } from "./fileIntoNewMailbox.js"
import { searchEmailsPages } from "./searchEmails.js"
import { sendEmail } from "./sendEmail.js"

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

// A /set that refuses a record throws, and carries the response it did get.
{
  const body = {
    sessionState: "session-1",
    methodResponses: [
      ["Email/set", { accountId: "acct1", newState: "s2",
        notCreated: { draft: { type: "invalidProperties", properties: ["subject"], description: "too long" } } }, "write"],
      ["EmailSubmission/set", { accountId: "acct1", newState: "sub2",
        notCreated: { send: { type: "invalidEmail" } } }, "send"],
    ],
  }
  const withAccounts = {
    ...session,
    primaryAccounts: {
      "urn:ietf:params:jmap:mail": "acct1",
      "urn:ietf:params:jmap:submission": "acct1",
    },
    capabilities: {
      "urn:ietf:params:jmap:core": {},
      "urn:ietf:params:jmap:mail": {},
      "urn:ietf:params:jmap:submission": {},
    },
  }
  const serve = (async (url: string | URL) =>
    new Response(JSON.stringify(String(url).endsWith("/api") ? body : withAccounts), {
      headers: { "Content-Type": "application/json" },
    })) as unknown as typeof fetch

  const c = new Client("https://example.com/.well-known/jmap", { fetch: serve })
  let thrown: unknown
  try {
    await sendEmail(c, {
      draftsMailboxId: "drafts",
      sentMailboxId: "sent",
      identityId: "id1",
      fromAddress: "me@example.com",
      toAddress: "you@example.com",
      subject: "Lunch",
      body: "Thursday?",
    })
  } catch (e) {
    thrown = e
  }
  assert(thrown instanceof SetErrors, `threw ${thrown}, not SetErrors`)
  const errs = thrown as SetErrors

  // The call the query does not return is checked too, so naming one call in
  // "_returns" does not stop the others from being looked at.
  assert(errs.failures.length === 2, `got ${errs.failures.length} failures, want 2`)
  assert(errs.failures[0].method === "Email/set", "the unreturned call was not checked")
  assert(errs.failures[0].key === "draft", "the failure is filed under the wrong key")

  const want = 'Email/set could not create "draft": invalidProperties [subject]: too long (and 1 more)'
  assert(errs.message === want, `message is ${errs.message}`)

  // The part of the response that did arrive is on the error, since it happened.
  const result = errs.result as { newState?: string }
  assert(result.newState === "sub2", "the response was not carried on the error")
}

// A search whose results do not fit in one request: the walk asks from where
// the last window ended, and the total ends it rather than a request for a
// window that is not there.
{
  const mailSession = {
    ...session,
    primaryAccounts: { "urn:ietf:params:jmap:mail": "acct1" },
    capabilities: { "urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {} },
  }
  const window = (position: number, ids: string[]) => ({
    sessionState: "s1",
    methodResponses: [
      ["Email/query", {
        accountId: "acct1", queryState: "q1", canCalculateChanges: false,
        position, total: 3, ids,
      }, "search"],
      ["Email/get", {
        accountId: "acct1", state: "s1", notFound: [],
        list: ids.map((id) => ({
          id, subject: `message ${id}`, receivedAt: "2026-09-04T09:00:00Z",
          from: [{ email: "someone@example.com" }],
        })),
      }, "fetch"],
    ],
  })
  const windows = [window(0, ["m1", "m2"]), window(2, ["m3"])]
  const asked: number[] = []
  const serve = (async (url: string | URL, init?: RequestInit) => {
    if (!String(url).endsWith("/api")) {
      return new Response(JSON.stringify(mailSession), {
        headers: { "Content-Type": "application/json" },
      })
    }
    const sent = JSON.parse(String(init?.body)) as { methodCalls: [string, { position: number }, string][] }
    asked.push(sent.methodCalls[0][1].position)
    const next = windows[asked.length - 1]
    assert(next !== undefined, `the walk asked for a window past the end, from ${asked.at(-1)}`)
    return new Response(JSON.stringify(next), { headers: { "Content-Type": "application/json" } })
  }) as unknown as typeof fetch

  const c = new Client("https://example.com/.well-known/jmap", { fetch: serve })
  const subjects: string[] = []
  for await (const page of searchEmailsPages(c, {
    phrase: "invoice",
    firstMailboxId: "mbx1",
    secondMailboxId: "mbx2",
    position: 0,
  })) {
    for (const email of page.fetch.list) subjects.push(email.subject ?? "")
  }

  assert(subjects.length === 3, `the walk found ${subjects.length} messages, want 3`)
  assert(asked.join(",") === "0,2", `the walk asked from ${asked.join(",")}, want 0,2`)
}

// A call the server would not run does not take the answers to the others
// with it: what the response did carry is on the error, as much of the result
// as the server answered.
{
  const mailSession = {
    ...session,
    primaryAccounts: { "urn:ietf:params:jmap:mail": "acct1" },
    capabilities: { "urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {} },
  }
  const serve = (async (url: string | URL, init?: RequestInit) => {
    if (!String(url).endsWith("/api")) {
      return new Response(JSON.stringify(mailSession), {
        headers: { "Content-Type": "application/json" },
      })
    }
    return new Response(JSON.stringify({
      sessionState: "s1",
      methodResponses: [
        ["Mailbox/set", { accountId: "acct1", newState: "m2", created: { box: { id: "mbx9" } } }, "make"],
        ["error", { type: "invalidResultReference" }, "file"],
      ],
    }), { headers: { "Content-Type": "application/json" } })
  }) as unknown as typeof fetch

  const c = new Client("https://example.com/.well-known/jmap", { fetch: serve })
  let thrown: unknown
  try {
    await fileIntoNewMailbox(c, { name: "Archive", emailId: "e1", fromMailboxId: "mbx1" })
  } catch (e) {
    thrown = e
  }
  assert(thrown instanceof MethodErrors, `threw ${thrown}, want MethodErrors`)
  const failed = thrown as MethodErrors
  assert(failed.errors.length === 1 && failed.errors[0].callId === "file",
    `the error names ${failed.errors.map((e) => e.callId).join(",")}, want the call that failed`)

  const partial = failed.result as Partial<FileIntoNewMailboxResult>
  assert(partial?.make?.newState === "m2",
    "the call that succeeded is not on the error")
  assert(partial?.file === undefined,
    "the call the server would not run was made up rather than left out")
}

console.log("ok")
