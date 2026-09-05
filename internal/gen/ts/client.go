package ts

import (
	"bytes"
	"fmt"
)

// ClientGenerator writes the runtime the generated queries call: a client, the
// request and response shapes, and the two levels at which JMAP fails.
//
// It is generated rather than published, so that a project using jmapc takes on
// no dependency at all. What it needs of the platform is fetch, which every
// runtime worth targeting now has.
type ClientGenerator struct{}

// Generate returns the source of client.ts.
func (g *ClientGenerator) Generate() ([]byte, error) {
	var buf bytes.Buffer
	writeHeader(&buf, "the jmapc runtime")
	fmt.Fprint(&buf, clientSource)
	return buf.Bytes(), nil
}

// clientSource is the runtime itself. It is a literal rather than something
// assembled, because none of it varies with the catalogue or the queries.
const clientSource = `import type { Account, Id, SetError } from "./types.js"

// A JMAP request: the capabilities it depends on, and the calls to make.
export interface Request {
  using: string[]
  methodCalls: Invocation[]
  createdIds?: { [creationId: Id]: Id }
}

// One method call or method response, which the wire format writes as the
// three-element array [name, arguments, callId].
export type Invocation = [string, unknown, string]

// A JMAP response: one entry per executed call, in the order the server ran
// them.
export interface Response {
  methodResponses: Invocation[]
  createdIds?: { [creationId: Id]: Id }
  sessionState: string
}

// A reference to a value in the response to an earlier call in the same
// request, which is what lets a chain of dependent calls cost one round trip.
export interface ResultReference {
  resultOf: string
  name: string
  path: string
}

// The session object: where to send requests, and what the server supports.
export interface Session {
  capabilities: { [uri: string]: unknown }
  accounts: { [id: Id]: Account }
  primaryAccounts: { [uri: string]: Id }
  username: string
  apiUrl: string
  downloadUrl: string
  uploadUrl: string
  eventSourceUrl: string
  state: string
}

// A request-level failure, where the server rejected the request whole. It
// carries the problem type of RFC 8620, Section 3.6.1.
export class RequestError extends Error {
  readonly status: number
  readonly type: string
  readonly detail?: string
  readonly limit?: string

  constructor(status: number, body: { type?: string; detail?: string; limit?: string; title?: string }) {
    const type = body.type ?? "about:blank"
    super(` + "`" + `jmapc: request failed with status ${status}: ${type}${body.detail ? ": " + body.detail : ""}` + "`" + `)
    this.name = "RequestError"
    this.status = status
    this.type = type
    this.detail = body.detail ?? body.title
    this.limit = body.limit
  }
}

// A method-level failure. JMAP runs the calls it can, so some of the response
// may still be usable; this names the method and call that failed rather than
// the bare "error" the wire format carries.
export class MethodError extends Error {
  readonly callId: string
  readonly methodName: string
  readonly type: string
  readonly description?: string
  readonly raw: Record<string, unknown>

  constructor(callId: string, methodName: string, raw: Record<string, unknown>) {
    const type = (raw.type as string) ?? "serverFail"
    super(` + "`" + `jmapc: ${methodName} (call "${callId}") failed: ${type}` + "`" + `)
    this.name = "MethodError"
    this.callId = callId
    this.methodName = methodName
    this.type = type
    this.description = raw.description as string | undefined
    this.raw = raw
  }
}

// Several method-level failures from one response.
//
// JMAP runs the calls it can, so the response is carried here too, and a
// generated query puts what it could read out of it on result: the calls the
// server answered, and nothing for the ones it would not run. Read it as a
// Partial of what the query returns.
export class MethodErrors extends Error {
  readonly errors: MethodError[]
  readonly response: Response
  result?: unknown

  constructor(errors: MethodError[], response: Response) {
    super(errors.length === 1 ? errors[0].message : ` + "`" + `jmapc: ${errors.length} method calls failed` + "`" + `)
    this.name = "MethodErrors"
    this.errors = errors
    this.response = response
  }
}

// One record a /set would not act on, and what the server said about it.
export interface SetFailure {
  // The method call that refused the record, such as "Email/set".
  method: string
  // The id of that call within the request.
  callId: string
  // The response property the failure was reported in, such as "notCreated".
  kind: string
  // The creation id or record id the failure is filed under.
  key: string
  // What the server said.
  error: SetError
}

// The records a request could not act on. A /set answers 200 and lists what it
// refused, so a caller that catches only a transport error sees success where
// there was none; generated code collects those refusals and throws this.
//
// The part of the response that did succeed is on result, since it happened.
export class SetErrors extends Error {
  readonly failures: SetFailure[]
  readonly result: unknown

  constructor(failures: SetFailure[], result: unknown) {
    super(describeSetFailures(failures))
    this.name = "SetErrors"
    this.failures = failures
    this.result = result
  }
}

// Turn a JMAP response property into the verb it denies, so that an error
// reads as prose rather than as a field name.
function setVerb(kind: string): string {
  switch (kind) {
    case "notCreated": return "create"
    case "notUpdated": return "update"
    case "notDestroyed": return "destroy"
    case "notCopied": return "copy"
  }
  return kind.startsWith("not") ? kind.slice(3) : kind
}

function describeSetFailure(f: SetFailure): string {
  let what = f.error.type ?? "unknown error"
  if (f.error.properties && f.error.properties.length > 0) {
    what += " [" + f.error.properties.join(", ") + "]"
  }
  if (f.error.description) {
    what += ": " + f.error.description
  }
  return f.method + " could not " + setVerb(f.kind) + " \"" + f.key + "\": " + what
}

function describeSetFailures(failures: SetFailure[]): string {
  if (failures.length === 0) return "no records failed"
  const first = describeSetFailure(failures[0])
  return failures.length === 1 ? first : first + " (and " + (failures.length - 1) + " more)"
}

// Record the failures one method call reported, keyed by the response property
// they arrived in. Called by generated code.
export function collectSetErrors(
  method: string,
  callId: string,
  groups: { [kind: string]: { [key: string]: SetError } | null | undefined },
  into: SetFailure[],
): void {
  for (const kind of Object.keys(groups).sort()) {
    const group = groups[kind]
    if (!group) continue
    for (const key of Object.keys(group).sort()) {
      into.push({ method, callId, kind, key, error: group[key] })
    }
  }
}

// How to authenticate. Given a token, the client sends it as a bearer token;
// given a function, it calls it for each request, which covers the schemes a
// token does not.
export type Auth = string | ((headers: Headers) => void | Promise<void>)

export interface ClientOptions {
  // How to authenticate.
  auth?: Auth
  // Sent on every request, for what a server asks of its clients beyond
  // authentication: a tenant, an API version, a trace id. Applied before auth,
  // so auth has the last word on the headers it sets.
  headers?: HeadersInit
  // Where to POST requests, if the session's apiUrl should not be used.
  apiUrl?: string
  // What to fetch with, for a runtime whose fetch is not the global one, or
  // for a test.
  fetch?: typeof fetch
  // Stop checking a request against the session before sending it. The checks
  // turn a wasted round trip into a local error, so leave them on unless a
  // server under-reports what it supports.
  skipPreflight?: boolean
}

// A client for one JMAP server. It caches the session, so a query costs one
// round trip rather than two.
export class Client {
  private readonly sessionUrl: string
  private readonly options: ClientOptions
  private cached?: Session

  constructor(sessionUrl: string, options: ClientOptions = {}) {
    this.sessionUrl = sessionUrl
    this.options = options
  }

  // The session, fetched on first use and cached afterwards.
  async session(): Promise<Session> {
    if (this.cached) return this.cached
    return this.refreshSession()
  }

  // Fetch the session again, replacing the cached copy. Call it when a
  // response reports a sessionState different from the one you hold.
  async refreshSession(): Promise<Session> {
    const res = await this.send(this.sessionUrl, { method: "GET" })
    if (!res.ok) throw await requestError(res)
    const session = (await res.json()) as Session
    if (!session.apiUrl) {
      throw new Error(` + "`" + `jmapc: session from ${this.sessionUrl} has no apiUrl` + "`" + `)
    }
    this.cached = session
    return session
  }

  // The account to use by default for a capability.
  async primaryAccountId(capability: string): Promise<Id> {
    const session = await this.session()
    const id = session.primaryAccounts[capability]
    if (!id) {
      throw new Error(` + "`" + `jmapc: session has no primary account for ${capability}` + "`" + `)
    }
    return id
  }

  // Send one request. A method-level failure throws MethodErrors; the response
  // is attached to it, since the calls that did run still have results.
  async request(req: Request): Promise<Response> {
    const apiUrl = this.options.apiUrl ?? (await this.session()).apiUrl
    if (!this.options.skipPreflight && !this.options.apiUrl) {
      await this.preflight(req)
    }
    const res = await this.send(apiUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json; charset=utf-8" },
      body: JSON.stringify(req),
    })
    if (!res.ok) throw await requestError(res)

    const response = (await res.json()) as Response
    const errors = methodErrors(req, response)
    if (errors.length > 0) throw new MethodErrors(errors, response)
    return response
  }

  // Reject a request the session already shows the server will not accept, so
  // that a missing capability surfaces without a round trip.
  private async preflight(req: Request): Promise<void> {
    const session = await this.session()
    for (const uri of req.using) {
      if (!(uri in session.capabilities)) {
        throw new RequestError(0, {
          type: "urn:ietf:params:jmap:error:unknownCapability",
          detail: ` + "`" + `server does not support ${uri}` + "`" + `,
        })
      }
    }
    const core = session.capabilities["urn:ietf:params:jmap:core"] as
      | { maxCallsInRequest?: number }
      | undefined
    const max = core?.maxCallsInRequest
    if (max && req.methodCalls.length > max) {
      throw new RequestError(0, {
        type: "urn:ietf:params:jmap:error:limit",
        limit: "maxCallsInRequest",
        detail: ` + "`" + `request has ${req.methodCalls.length} method calls, server allows ${max}` + "`" + `,
      })
    }
  }

  private async send(url: string, init: RequestInit): Promise<globalThis.Response> {
    const headers = new Headers(init.headers)
    headers.set("Accept", "application/json")
    // forEach rather than iteration, which would require dom.iterable in the
    // caller's tsconfig; the client should compile under a plain dom lib.
    new Headers(this.options.headers).forEach((value, key) => headers.set(key, value))
    const auth = this.options.auth
    if (typeof auth === "string") {
      headers.set("Authorization", ` + "`" + `Bearer ${auth}` + "`" + `)
    } else if (auth) {
      await auth(headers)
    }
    const f = this.options.fetch ?? fetch
    return f(url, { ...init, headers })
  }
}

// Decode the response to one method call, throwing where the server reported
// an error for it.
export function decode<T>(req: Request, res: Response, callId: string): T {
  for (const [name, args, id] of res.methodResponses) {
    if (id !== callId) continue
    if (name === "error") {
      throw new MethodError(callId, requestedMethod(req, callId), args as Record<string, unknown>)
    }
    return args as T
  }
  const ids = res.methodResponses.map(([, , id]) => ` + "`" + `"${id}"` + "`" + `).join(", ") || "no results"
  throw new Error(` + "`" + `jmapc: response has no result for call "${callId}" (response has ${ids})` + "`" + `)
}

// Whether the server answered this call with a result. A call it would not run
// is answered with an error instead, and one it never reached is not in the
// response at all; neither is something to read.
export function answered(res: Response, callId: string): boolean {
  return res.methodResponses.some(([name, , id]) => id === callId && name !== "error")
}

// Every method-level error in a response.
function methodErrors(req: Request, res: Response): MethodError[] {
  const errors: MethodError[] = []
  for (const [name, args, id] of res.methodResponses) {
    if (name === "error") {
      errors.push(new MethodError(id, requestedMethod(req, id), args as Record<string, unknown>))
    }
  }
  return errors
}

// The method the client asked for under a call id, which the wire format
// replaces with the literal name "error" when it fails.
function requestedMethod(req: Request, callId: string): string {
  for (const [name, , id] of req.methodCalls) {
    if (id === callId) return name
  }
  return "error"
}

// Turn a non-2xx response into a RequestError, decoding the RFC 7807 problem
// details document where the server sent one.
async function requestError(res: globalThis.Response): Promise<RequestError> {
  const type = res.headers.get("Content-Type") ?? ""
  if (type.startsWith("application/problem+json") || type.startsWith("application/json")) {
    try {
      return new RequestError(res.status, await res.json())
    } catch {
      // Fall through to the plain-text form.
    }
  }
  return new RequestError(res.status, { detail: (await res.text()).trim() })
}
`
