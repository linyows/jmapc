//! Hand-written, unlike everything under src/jmapq: what the generated runtime
//! does when it runs, which the compiler cannot say. That the headers go out,
//! that auth wins over them, that the session is cached, and that a /set which
//! answers 200 and refuses a record is still an error.

use std::sync::Mutex;

use futures_lite::future::block_on;
use serde_json::json;

use jmapc_example::jmapq::create_mailbox::{create_mailbox, CreateMailboxParams};
use jmapc_example::jmapq::file_into_new_mailbox::{
    file_into_new_mailbox, FileIntoNewMailboxParams, FileIntoNewMailboxResult,
};
use jmapc_example::jmapq::search_emails::{search_emails_pages, SearchEmailsParams};
use jmapc_example::jmapq::{
    Auth, Client, ClientOptions, Error, HttpRequest, HttpResponse, Transport, TransportError,
};

/// A transport that records what it was given and answers from a queue, so a
/// test can say what the server sends back without a server.
#[derive(Default)]
struct Stub {
    sent: Mutex<Vec<HttpRequest>>,
    replies: Mutex<Vec<serde_json::Value>>,
}

impl Stub {
    fn new(replies: Vec<serde_json::Value>) -> Self {
        Self {
            sent: Mutex::new(Vec::new()),
            replies: Mutex::new(replies),
        }
    }

    /// Answer one request. The work happens before the future is made, so
    /// nothing holds a lock across an await.
    fn handle(&self, req: HttpRequest) -> HttpResponse {
        self.sent.lock().unwrap().push(req);
        let mut replies = self.replies.lock().unwrap();
        let body = if replies.len() > 1 {
            replies.remove(0)
        } else {
            replies[0].clone()
        };
        HttpResponse {
            status: 200,
            content_type: "application/json".to_string(),
            body: serde_json::to_vec(&body).unwrap(),
        }
    }

    fn sent(&self) -> Vec<HttpRequest> {
        self.sent.lock().unwrap().clone()
    }
}

// The impl is on the reference rather than on Stub itself, so that a test can
// still read what was sent after handing the transport to a client.
impl Transport for &Stub {
    async fn send(&self, req: HttpRequest) -> Result<HttpResponse, TransportError> {
        Ok(self.handle(req))
    }
}

/// The session every test starts from.
fn session() -> serde_json::Value {
    json!({
        "apiUrl": "https://example.com/jmap/api",
        "accounts": {},
        "primaryAccounts": {"urn:ietf:params:jmap:mail": "acct1"},
        "capabilities": {
            "urn:ietf:params:jmap:core": {},
            "urn:ietf:params:jmap:mail": {},
        },
        "username": "someone@example.com",
        "state": "s1",
    })
}

/// The header a request carried, matched as HTTP matches a name.
fn header<'a>(req: &'a HttpRequest, name: &str) -> Option<&'a str> {
    req.headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case(name))
        .map(|(_, v)| v.as_str())
}

#[test]
fn headers_reach_the_server_and_auth_has_the_last_word() {
    let stub = Stub::new(vec![session()]);
    let client = Client::with_options(
        "https://example.com/.well-known/jmap",
        &stub,
        ClientOptions {
            auth: Some(Auth::Bearer("tok".to_string())),
            headers: vec![
                ("X-Tenant".to_string(), "acme".to_string()),
                ("Authorization".to_string(), "not this one".to_string()),
            ],
            ..ClientOptions::default()
        },
    );

    let s = block_on(client.session()).expect("the session should be readable");
    assert_eq!(s.api_url, "https://example.com/jmap/api");

    let sent = stub.sent();
    assert_eq!(sent.len(), 1);
    assert_eq!(sent[0].method, "GET");
    assert_eq!(header(&sent[0], "x-tenant"), Some("acme"));
    assert_eq!(header(&sent[0], "accept"), Some("application/json"));
    assert_eq!(header(&sent[0], "authorization"), Some("Bearer tok"));
}

#[test]
fn the_session_is_fetched_once() {
    let stub = Stub::new(vec![session()]);
    let client = Client::new("https://example.com/.well-known/jmap", &stub);

    block_on(client.session()).unwrap();
    block_on(client.session()).unwrap();
    assert_eq!(stub.sent().len(), 1, "the session should be cached");

    block_on(client.refresh_session()).unwrap();
    assert_eq!(stub.sent().len(), 2, "refreshing should ask again");
}

#[test]
fn a_query_sends_the_request_it_was_generated_from() {
    let stub = Stub::new(vec![
        session(),
        json!({
            "methodResponses": [["Mailbox/set", {
                "accountId": "acct1",
                "newState": "s2",
                "created": {"new": {"id": "mbx9"}},
            }, "create"]],
            "sessionState": "s1",
        }),
    ]);
    let client = Client::with_bearer_token("https://example.com/.well-known/jmap", &stub, "tok");

    let res = block_on(create_mailbox(
        &client,
        CreateMailboxParams {
            name: "Receipts".to_string(),
        },
    ))
    .expect("the mailbox should be created");
    assert_eq!(res.new_state, "s2");
    let created = res.created.expect("the mailbox that was created");
    assert_eq!(created["new"].as_ref().unwrap().id.as_deref(), Some("mbx9"));

    let sent = stub.sent();
    assert_eq!(sent.len(), 2);
    let body: serde_json::Value = serde_json::from_slice(sent[1].body.as_ref().unwrap()).unwrap();
    assert_eq!(
        body["using"],
        json!(["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"])
    );
    assert_eq!(body["methodCalls"][0][0], "Mailbox/set");
    assert_eq!(body["methodCalls"][0][1]["accountId"], "acct1");
    assert_eq!(
        body["methodCalls"][0][1]["create"]["new"]["name"],
        "Receipts"
    );
    assert_eq!(body["methodCalls"][0][2], "create");
}

#[test]
fn a_refused_record_is_an_error_even_though_the_status_was_200() {
    let stub = Stub::new(vec![
        session(),
        json!({
            "methodResponses": [["Mailbox/set", {
                "accountId": "acct1",
                "newState": "s2",
                "notCreated": {"new": {
                    "type": "invalidProperties",
                    "properties": ["name"],
                    "description": "a mailbox of that name is already there",
                }},
            }, "create"]],
            "sessionState": "s1",
        }),
    ]);
    let client = Client::new("https://example.com/.well-known/jmap", &stub);

    let err = block_on(create_mailbox(
        &client,
        CreateMailboxParams {
            name: "Receipts".to_string(),
        },
    ))
    .expect_err("a refused record should not read as success");

    let Error::Set(refused) = err else {
        panic!("expected the refusal to be reported as Error::Set");
    };
    assert_eq!(refused.failures.len(), 1);
    let failure = &refused.failures[0];
    assert_eq!(failure.method, "Mailbox/set");
    assert_eq!(failure.call_id, "create");
    assert_eq!(failure.kind, "notCreated");
    assert_eq!(failure.key, "new");
    assert_eq!(failure.error.r#type.as_deref(), Some("invalidProperties"));
    assert!(
        refused.to_string().contains("could not create"),
        "the message should read as prose, not as a field name: {refused}"
    );

    // The rest of the response happened, and is still there to be read.
    let result = refused
        .result::<jmapc_example::jmapq::types::MailboxSetResponse>()
        .expect("the response the request did get");
    assert_eq!(result.new_state, "s2");
}

#[test]
fn a_capability_the_session_does_not_have_never_leaves_the_client() {
    let stub = Stub::new(vec![json!({
        "apiUrl": "https://example.com/jmap/api",
        "accounts": {},
        "primaryAccounts": {"urn:ietf:params:jmap:mail": "acct1"},
        "capabilities": {"urn:ietf:params:jmap:core": {}},
        "username": "someone@example.com",
        "state": "s1",
    })]);
    let client = Client::new("https://example.com/.well-known/jmap", &stub);

    let err = block_on(create_mailbox(
        &client,
        CreateMailboxParams {
            name: "Receipts".to_string(),
        },
    ))
    .expect_err("a capability the server does not advertise should stop here");

    let Error::Request(problem) = err else {
        panic!("expected a request-level error");
    };
    assert_eq!(
        problem.r#type,
        "urn:ietf:params:jmap:error:unknownCapability"
    );
    assert_eq!(
        stub.sent().len(),
        1,
        "only the session should have been fetched"
    );
}

/// One window of a search, and the emails in it, as the two calls of
/// SearchEmails ask for them.
fn window(position: u64, total: u64, ids: &[&str]) -> serde_json::Value {
    let list: Vec<serde_json::Value> = ids
        .iter()
        .map(|id| {
            json!({
                "id": id,
                "subject": format!("message {id}"),
                "from": [{"email": "someone@example.com"}],
                "receivedAt": "2026-09-04T09:00:00Z",
            })
        })
        .collect();
    json!({
        "methodResponses": [
            ["Email/query", {
                "accountId": "acct1",
                "queryState": "q1",
                "canCalculateChanges": false,
                "position": position,
                "total": total,
                "limit": 50,
                "ids": ids,
            }, "search"],
            ["Email/get", {"accountId": "acct1", "state": "s1", "list": list, "notFound": []}, "fetch"],
        ],
        "sessionState": "s1",
    })
}

#[test]
fn a_walk_asks_for_each_window_in_turn() {
    let stub = Stub::new(vec![
        session(),
        window(0, 3, &["m1", "m2"]),
        window(2, 3, &["m3"]),
    ]);
    let client = Client::new("https://example.com/.well-known/jmap", &stub);

    let mut pages = search_emails_pages(SearchEmailsParams {
        phrase: "invoice".to_string(),
        first_mailbox_id: "mbx1".to_string(),
        second_mailbox_id: "mbx2".to_string(),
        position: 0,
    });
    let mut subjects = Vec::new();
    while let Some(page) = block_on(pages.next(&client)).expect("the walk should hold") {
        for email in &page.fetch.list {
            subjects.push(email.subject.clone().unwrap_or_default());
        }
    }

    assert_eq!(subjects, vec!["message m1", "message m2", "message m3"]);
    // The session, and then one request per window: the total ends the walk
    // rather than a request for a window that is not there.
    let sent = stub.sent();
    assert_eq!(sent.len(), 3);
    let first: serde_json::Value = serde_json::from_slice(sent[1].body.as_ref().unwrap()).unwrap();
    let second: serde_json::Value = serde_json::from_slice(sent[2].body.as_ref().unwrap()).unwrap();
    assert_eq!(first["methodCalls"][0][1]["position"], 0);
    assert_eq!(second["methodCalls"][0][1]["position"], 2);
}

/// A call the server would not run does not take the answers to the others
/// with it. JMAP runs the calls it can, so what the response did carry is on
/// the error, with the call that failed left at its default.
#[test]
fn the_calls_that_ran_are_on_the_error() {
    let stub = Stub::new(vec![
        session(),
        json!({
            "methodResponses": [
                ["Mailbox/set", {
                    "accountId": "acct1",
                    "newState": "m2",
                    "created": {"box": {"id": "mbx9"}},
                }, "make"],
                ["error", {"type": "invalidResultReference"}, "file"],
            ],
            "sessionState": "s1",
        }),
    ]);
    let client = Client::new("https://example.com/.well-known/jmap", &stub);

    let err = block_on(file_into_new_mailbox(
        &client,
        FileIntoNewMailboxParams {
            name: "Archive".to_string(),
            email_id: "e1".to_string(),
            from_mailbox_id: "mbx1".to_string(),
        },
        None,
    ))
    .expect_err("a call the server would not run is still a failure");

    let Error::Method(failed) = err else {
        panic!("expected the failure to be reported as Error::Method");
    };
    assert_eq!(failed.errors.len(), 1);
    assert_eq!(failed.errors[0].call_id, "file");

    let out = failed
        .result::<FileIntoNewMailboxResult>()
        .expect("what the calls that ran answered with");
    assert_eq!(out.make.new_state, "m2");
    assert!(
        out.make.created.is_some(),
        "the mailbox the server created is not in the result"
    );
    assert_eq!(
        out.file,
        Default::default(),
        "the call the server would not run should be left at its default"
    );
}
