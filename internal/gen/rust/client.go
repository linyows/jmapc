package rust

import (
	"bytes"
	"fmt"
)

// ClientGenerator writes the runtime the generated queries call: a client, the
// request and response shapes, and the levels at which JMAP fails.
//
// It is generated rather than published, so that a project using jmapc takes on
// no jmapc dependency at all. What it asks of the crate around it is serde and
// serde_json; how the bytes travel is a Transport the caller supplies, which
// keeps an HTTP stack, a TLS backend and an async runtime out of the generated
// code.
type ClientGenerator struct{}

// Generate returns the source of client.rs.
func (g *ClientGenerator) Generate() ([]byte, error) {
	var buf bytes.Buffer
	writeHeader(&buf, "the jmapc runtime")
	fmt.Fprint(&buf, clientSource)
	return finish(&buf), nil
}

// clientSource is the runtime itself. It is a literal rather than something
// assembled, because none of it varies with the catalogue or the queries.
const clientSource = `use std::any::Any;
use std::collections::BTreeMap;
use std::fmt;
use std::future::Future;
use std::sync::Mutex;

use serde::{Deserialize, Serialize};

use super::types::{Account, Id, SetError};

/// A JMAP request: the capabilities it depends on, and the calls to make.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Request {
    pub using: Vec<String>,
    pub method_calls: Vec<Invocation>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub created_ids: Option<BTreeMap<Id, Id>>,
}

/// One method call or method response, which the wire format writes as the
/// three-element array [name, arguments, callId].
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Invocation(pub String, pub serde_json::Value, pub String);

/// A JMAP response: one entry per executed call, in the order the server ran
/// them.
#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Response {
    #[serde(default)]
    pub method_responses: Vec<Invocation>,
    #[serde(default)]
    pub created_ids: Option<BTreeMap<Id, Id>>,
    #[serde(default)]
    pub session_state: String,
}

/// A reference to a value in the response to an earlier call in the same
/// request, which is what lets a chain of dependent calls cost one round trip.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ResultReference {
    pub result_of: String,
    pub name: String,
    pub path: String,
}

/// The session object: where to send requests, and what the server supports.
#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Session {
    #[serde(default)]
    pub capabilities: BTreeMap<String, serde_json::Value>,
    #[serde(default)]
    pub accounts: BTreeMap<Id, Account>,
    #[serde(default)]
    pub primary_accounts: BTreeMap<String, Id>,
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub api_url: String,
    #[serde(default)]
    pub download_url: String,
    #[serde(default)]
    pub upload_url: String,
    #[serde(default)]
    pub event_source_url: String,
    #[serde(default)]
    pub state: String,
}

/// One HTTP exchange, as the client hands it to a Transport. The generated code
/// speaks JMAP and nothing else, so the request is described rather than sent.
#[derive(Debug, Clone, PartialEq)]
pub struct HttpRequest {
    /// The method, which is GET for the session and POST for everything else.
    pub method: &'static str,
    /// The absolute URL to send to.
    pub url: String,
    /// The headers to send, each already in the form the wire wants.
    pub headers: Vec<(String, String)>,
    /// The body, for a request that has one.
    pub body: Option<Vec<u8>>,
}

/// What a Transport answers with.
#[derive(Debug, Clone, PartialEq)]
pub struct HttpResponse {
    /// The status code.
    pub status: u16,
    /// The Content-Type header, which decides how a failure is read. An empty
    /// string stands for a response that did not carry one.
    pub content_type: String,
    /// The body, as it arrived.
    pub body: Vec<u8>,
}

/// The error a Transport reports when it could not deliver the request at all.
pub type TransportError = Box<dyn std::error::Error + Send + Sync>;

/// How the bytes travel. Implement it over whichever HTTP client the program
/// already has, and the generated code takes on no HTTP stack, no TLS backend
/// and no async runtime of its own.
///
/// An implementation writes the method as an ordinary async fn. The future it
/// returns has to be Send, which is what lets a query be spawned.
///
/// Authentication that a bearer token does not cover — a signature over the
/// request, a token refreshed on expiry — belongs here too, since this is the
/// last thing to see the request before it goes.
pub trait Transport {
    fn send(
        &self,
        req: HttpRequest,
    ) -> impl Future<Output = Result<HttpResponse, TransportError>> + Send;
}

/// How to authenticate. A bearer token is sent as one; anything else belongs in
/// the Transport, which sees every request on its way out.
#[derive(Debug, Clone, PartialEq)]
pub enum Auth {
    Bearer(String),
}

/// What a client may be told beyond where the session lives.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct ClientOptions {
    /// How to authenticate.
    pub auth: Option<Auth>,
    /// Sent on every request, for what a server asks of its clients beyond
    /// authentication: a tenant, an API version, a trace id. Applied before
    /// auth, so auth has the last word on the headers it sets.
    pub headers: Vec<(String, String)>,
    /// Where to POST requests, if the session's apiUrl should not be used.
    pub api_url: Option<String>,
    /// Stop checking a request against the session before sending it. The
    /// checks turn a wasted round trip into a local error, so leave them on
    /// unless a server under-reports what it supports.
    pub skip_preflight: bool,
}

/// A request-level failure, where the server rejected the request whole. It
/// carries the problem type of RFC 8620, Section 3.6.1.
#[derive(Debug, Clone, PartialEq)]
pub struct RequestError {
    pub status: u16,
    pub r#type: String,
    pub detail: Option<String>,
    pub limit: Option<String>,
}

impl fmt::Display for RequestError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "request failed with status {}: {}",
            self.status, self.r#type
        )?;
        if let Some(detail) = &self.detail {
            write!(f, ": {detail}")?;
        }
        Ok(())
    }
}

/// The RFC 7807 problem details document a server sends with a failure.
#[derive(Debug, Clone, Default, Deserialize)]
struct Problem {
    r#type: Option<String>,
    detail: Option<String>,
    limit: Option<String>,
    title: Option<String>,
}

/// A method-level failure. JMAP runs the calls it can, so some of the response
/// may still be usable; this names the method and call that failed rather than
/// the bare "error" the wire format carries.
#[derive(Debug, Clone, PartialEq)]
pub struct MethodError {
    pub call_id: String,
    pub method_name: String,
    pub r#type: String,
    pub description: Option<String>,
    pub raw: serde_json::Value,
}

impl MethodError {
    fn new(call_id: &str, method_name: &str, raw: serde_json::Value) -> Self {
        let text = |key: &str| raw.get(key).and_then(|v| v.as_str()).map(str::to_string);
        Self {
            call_id: call_id.to_string(),
            method_name: method_name.to_string(),
            r#type: text("type").unwrap_or_else(|| "serverFail".to_string()),
            description: text("description"),
            raw,
        }
    }
}

impl fmt::Display for MethodError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "{} (call {:?}) failed: {}",
            self.method_name, self.call_id, self.r#type
        )
    }
}

/// The method-level failures in one response, together with the response
/// itself, since the calls that did run still have results.
#[derive(Debug, Clone, PartialEq)]
pub struct MethodErrors {
    pub errors: Vec<MethodError>,
    pub response: Response,
}

impl fmt::Display for MethodErrors {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self.errors.split_first() {
            None => write!(f, "no method calls failed"),
            Some((first, [])) => write!(f, "{first}"),
            Some((first, rest)) => write!(f, "{first} (and {} more)", rest.len()),
        }
    }
}

/// One record a /set would not act on, and what the server said about it.
#[derive(Debug, Clone, PartialEq)]
pub struct SetFailure {
    /// The method call that refused the record, such as "Email/set".
    pub method: String,
    /// The id of that call within the request.
    pub call_id: String,
    /// The response property the failure was reported in, such as
    /// "notCreated".
    pub kind: String,
    /// The creation id or record id the failure is filed under.
    pub key: String,
    /// What the server said.
    pub error: SetError,
}

impl fmt::Display for SetFailure {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let mut what = self
            .error
            .r#type
            .clone()
            .unwrap_or_else(|| "unknown error".to_string());
        if let Some(properties) = &self.error.properties {
            if !properties.is_empty() {
                what += &format!(" [{}]", properties.join(", "));
            }
        }
        if let Some(description) = &self.error.description {
            what += &format!(": {description}");
        }
        write!(
            f,
            "{} could not {} {:?}: {}",
            self.method,
            set_verb(&self.kind),
            self.key,
            what
        )
    }
}

/// Turn a JMAP response property into the verb it denies, so that an error
/// reads as prose rather than as a field name.
fn set_verb(kind: &str) -> String {
    match kind {
        "notCreated" => "create".to_string(),
        "notUpdated" => "update".to_string(),
        "notDestroyed" => "destroy".to_string(),
        "notCopied" => "copy".to_string(),
        _ => kind.strip_prefix("not").unwrap_or(kind).to_string(),
    }
}

/// The records a request could not act on. A /set answers 200 and lists what it
/// refused, so a caller that looks only at the transport sees success where
/// there was none; generated code collects those refusals and returns this.
///
/// The part of the response that did succeed is kept, since it happened. Ask
/// for it back with the type the query returns.
pub struct SetErrors {
    pub failures: Vec<SetFailure>,
    result: Box<dyn Any + Send + Sync>,
}

impl SetErrors {
    pub fn new(failures: Vec<SetFailure>, result: impl Any + Send + Sync) -> Self {
        Self {
            failures,
            result: Box::new(result),
        }
    }

    /// The response the request did get, for the calls that were not refused.
    /// The type is the one the generated function would have returned.
    pub fn result<T: Any>(&self) -> Option<&T> {
        self.result.downcast_ref::<T>()
    }
}

impl fmt::Debug for SetErrors {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("SetErrors")
            .field("failures", &self.failures)
            .finish_non_exhaustive()
    }
}

impl fmt::Display for SetErrors {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self.failures.split_first() {
            None => write!(f, "no records failed"),
            Some((first, [])) => write!(f, "{first}"),
            Some((first, rest)) => write!(f, "{first} (and {} more)", rest.len()),
        }
    }
}

/// Everything that can go wrong between writing a query and holding its answer.
#[derive(Debug)]
pub enum Error {
    /// The server rejected the request whole.
    Request(RequestError),
    /// One or more method calls failed.
    Method(MethodErrors),
    /// The server refused to act on records a /set named.
    Set(SetErrors),
    /// The response did not have the shape the query asks for.
    Decode(serde_json::Error),
    /// The response carries no result for a call the request made.
    Missing(String),
    /// The session does not say something the client needs to know.
    Session(String),
    /// The transport could not deliver the request.
    Transport(TransportError),
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "jmapc: ")?;
        match self {
            Error::Request(e) => write!(f, "{e}"),
            Error::Method(e) => write!(f, "{e}"),
            Error::Set(e) => write!(f, "{e}"),
            Error::Decode(e) => write!(f, "the response could not be read: {e}"),
            Error::Missing(call_id) => write!(f, "response has no result for call {call_id:?}"),
            Error::Session(what) => write!(f, "{what}"),
            Error::Transport(e) => write!(f, "the request could not be sent: {e}"),
        }
    }
}

impl std::error::Error for Error {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Error::Decode(e) => Some(e),
            Error::Transport(e) => Some(e.as_ref()),
            _ => None,
        }
    }
}

/// A client for one JMAP server. It caches the session, so a query costs one
/// round trip rather than two.
pub struct Client<T> {
    session_url: String,
    transport: T,
    options: ClientOptions,
    cached: Mutex<Option<Session>>,
}

impl<T: Transport> Client<T> {
    /// A client that sends no credentials, for a server that wants them from
    /// the Transport instead.
    pub fn new(session_url: impl Into<String>, transport: T) -> Self {
        Self::with_options(session_url, transport, ClientOptions::default())
    }

    /// A client that sends a bearer token.
    pub fn with_bearer_token(
        session_url: impl Into<String>,
        transport: T,
        token: impl Into<String>,
    ) -> Self {
        Self::with_options(
            session_url,
            transport,
            ClientOptions {
                auth: Some(Auth::Bearer(token.into())),
                ..ClientOptions::default()
            },
        )
    }

    pub fn with_options(
        session_url: impl Into<String>,
        transport: T,
        options: ClientOptions,
    ) -> Self {
        Self {
            session_url: session_url.into(),
            transport,
            options,
            cached: Mutex::new(None),
        }
    }

    /// The session, fetched on first use and cached afterwards.
    pub async fn session(&self) -> Result<Session, Error> {
        if let Some(session) = self.cached_session() {
            return Ok(session);
        }
        self.refresh_session().await
    }

    /// Fetch the session again, replacing the cached copy. Call it when a
    /// response reports a sessionState different from the one you hold.
    pub async fn refresh_session(&self) -> Result<Session, Error> {
        let res = self
            .send(HttpRequest {
                method: "GET",
                url: self.session_url.clone(),
                headers: Vec::new(),
                body: None,
            })
            .await?;
        if !is_ok(res.status) {
            return Err(Error::Request(request_error(res)));
        }
        let session: Session = serde_json::from_slice(&res.body).map_err(Error::Decode)?;
        if session.api_url.is_empty() {
            return Err(Error::Session(format!(
                "session from {} has no apiUrl",
                self.session_url
            )));
        }
        *self.lock() = Some(session.clone());
        Ok(session)
    }

    /// The account to use by default for a capability.
    pub async fn primary_account_id(&self, capability: &str) -> Result<Id, Error> {
        let session = self.session().await?;
        match session.primary_accounts.get(capability) {
            Some(id) if !id.is_empty() => Ok(id.clone()),
            _ => Err(Error::Session(format!(
                "session has no primary account for {capability}"
            ))),
        }
    }

    /// Send one request. A method-level failure is an Error::Method; the
    /// response comes with it, since the calls that did run still have results.
    pub async fn request(&self, req: &Request) -> Result<Response, Error> {
        let api_url = match &self.options.api_url {
            Some(url) => url.clone(),
            None => self.session().await?.api_url,
        };
        if !self.options.skip_preflight && self.options.api_url.is_none() {
            self.preflight(req).await?;
        }
        let body = serde_json::to_vec(req).map_err(Error::Decode)?;
        let res = self
            .send(HttpRequest {
                method: "POST",
                url: api_url,
                headers: vec![(
                    "Content-Type".to_string(),
                    "application/json; charset=utf-8".to_string(),
                )],
                body: Some(body),
            })
            .await?;
        if !is_ok(res.status) {
            return Err(Error::Request(request_error(res)));
        }
        let response: Response = serde_json::from_slice(&res.body).map_err(Error::Decode)?;
        let errors = method_errors(req, &response);
        if !errors.is_empty() {
            return Err(Error::Method(MethodErrors { errors, response }));
        }
        Ok(response)
    }

    /// Reject a request the session already shows the server will not accept,
    /// so that a missing capability surfaces without a round trip.
    async fn preflight(&self, req: &Request) -> Result<(), Error> {
        let session = self.session().await?;
        for uri in &req.using {
            if !session.capabilities.contains_key(uri) {
                return Err(Error::Request(RequestError {
                    status: 0,
                    r#type: "urn:ietf:params:jmap:error:unknownCapability".to_string(),
                    detail: Some(format!("server does not support {uri}")),
                    limit: None,
                }));
            }
        }
        let max = session
            .capabilities
            .get("urn:ietf:params:jmap:core")
            .and_then(|c| c.get("maxCallsInRequest"))
            .and_then(|v| v.as_u64());
        if let Some(max) = max {
            if max > 0 && req.method_calls.len() as u64 > max {
                return Err(Error::Request(RequestError {
                    status: 0,
                    r#type: "urn:ietf:params:jmap:error:limit".to_string(),
                    detail: Some(format!(
                        "request has {} method calls, server allows {max}",
                        req.method_calls.len()
                    )),
                    limit: Some("maxCallsInRequest".to_string()),
                }));
            }
        }
        Ok(())
    }

    async fn send(&self, mut req: HttpRequest) -> Result<HttpResponse, Error> {
        set_header(&mut req.headers, "Accept", "application/json");
        for (name, value) in &self.options.headers {
            set_header(&mut req.headers, name, value);
        }
        if let Some(Auth::Bearer(token)) = &self.options.auth {
            set_header(
                &mut req.headers,
                "Authorization",
                &format!("Bearer {token}"),
            );
        }
        self.transport.send(req).await.map_err(Error::Transport)
    }

    fn cached_session(&self) -> Option<Session> {
        self.lock().clone()
    }

    /// The lock over the cached session. A poisoned lock holds a session and
    /// nothing else, so what is behind it is still worth having.
    fn lock(&self) -> std::sync::MutexGuard<'_, Option<Session>> {
        self.cached.lock().unwrap_or_else(|e| e.into_inner())
    }
}

/// Replace a header, or add it, matching the name as HTTP does.
fn set_header(headers: &mut Vec<(String, String)>, name: &str, value: &str) {
    if let Some(header) = headers
        .iter_mut()
        .find(|(k, _)| k.eq_ignore_ascii_case(name))
    {
        header.1 = value.to_string();
        return;
    }
    headers.push((name.to_string(), value.to_string()));
}

fn is_ok(status: u16) -> bool {
    (200..300).contains(&status)
}

/// Turn a non-2xx response into a RequestError, reading the RFC 7807 problem
/// details document where the server sent one.
fn request_error(res: HttpResponse) -> RequestError {
    let json = res.content_type.starts_with("application/problem+json")
        || res.content_type.starts_with("application/json");
    if json {
        if let Ok(problem) = serde_json::from_slice::<Problem>(&res.body) {
            return RequestError {
                status: res.status,
                r#type: problem.r#type.unwrap_or_else(|| "about:blank".to_string()),
                detail: problem.detail.or(problem.title),
                limit: problem.limit,
            };
        }
    }
    let text = String::from_utf8_lossy(&res.body).trim().to_string();
    RequestError {
        status: res.status,
        r#type: "about:blank".to_string(),
        detail: if text.is_empty() { None } else { Some(text) },
        limit: None,
    }
}

/// Every method-level error in a response.
fn method_errors(req: &Request, res: &Response) -> Vec<MethodError> {
    let mut errors = Vec::new();
    for Invocation(name, args, id) in &res.method_responses {
        if name == "error" {
            errors.push(MethodError::new(
                id,
                &requested_method(req, id),
                args.clone(),
            ));
        }
    }
    errors
}

/// The method the client asked for under a call id, which the wire format
/// replaces with the literal name "error" when it fails.
fn requested_method(req: &Request, call_id: &str) -> String {
    for Invocation(name, _, id) in &req.method_calls {
        if id == call_id {
            return name.clone();
        }
    }
    "error".to_string()
}

/// Read the response to one method call as the type the query asks for.
/// Called by generated code.
pub fn decode<T: serde::de::DeserializeOwned>(
    req: &Request,
    res: &Response,
    call_id: &str,
) -> Result<T, Error> {
    for Invocation(name, args, id) in &res.method_responses {
        if id != call_id {
            continue;
        }
        if name == "error" {
            return Err(Error::Method(MethodErrors {
                errors: vec![MethodError::new(
                    call_id,
                    &requested_method(req, call_id),
                    args.clone(),
                )],
                response: res.clone(),
            }));
        }
        return serde_json::from_value(args.clone()).map_err(Error::Decode);
    }
    Err(Error::Missing(call_id.to_string()))
}

/// Record the failures one method call reported, keyed by the response property
/// they arrived in. Called by generated code.
pub fn collect_set_errors(
    method: &str,
    call_id: &str,
    groups: &[(&str, Option<&BTreeMap<String, SetError>>)],
    into: &mut Vec<SetFailure>,
) {
    let mut ordered: Vec<_> = groups.iter().collect();
    ordered.sort_by_key(|(kind, _)| *kind);
    for (kind, group) in ordered {
        let Some(group) = group else { continue };
        for (key, error) in group.iter() {
            into.push(SetFailure {
                method: method.to_string(),
                call_id: call_id.to_string(),
                kind: kind.to_string(),
                key: key.clone(),
                error: error.clone(),
            });
        }
    }
}
`
