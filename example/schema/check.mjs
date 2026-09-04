// What the JSON Schema does when a validator reads it, which Go's tests cannot
// say: that it accepts every query in this repository and refuses the mistakes
// jmapc claims to catch. It throws on a failure, and Node exits non-zero, so it
// needs no test framework.
//
// Run it over a schema "jmapc schema" has written:
//
//     jmapc schema -out example/schema/jmapc.schema.json
//     cd example/schema && npm install && node check.mjs
import Ajv from "ajv"
import { readFileSync, readdirSync } from "node:fs"
import { join } from "node:path"

const schemaPath = process.argv[2] ?? "./jmapc.schema.json"
const schema = JSON.parse(readFileSync(schemaPath, "utf8"))

// The schema describes documents rather than constraining itself, so the
// members an editor reads for its own purposes are none of Ajv's business.
const ajv = new Ajv({ strict: false, allErrors: true })
const validate = ajv.compile(schema)

const failures = []

function accepts(name, doc) {
  if (!validate(doc)) {
    failures.push(`${name} should be accepted: ${ajv.errorsText(validate.errors)}`)
  }
}

function refuses(name, doc) {
  if (validate(doc)) {
    failures.push(`${name} should be refused`)
  }
}

// Every query the example holds is one jmapc generates a client from, so a
// schema that reports any of them is wrong about the language rather than about
// the query.
const dir = "../queries"
for (const file of readdirSync(dir).filter((f) => f.endsWith(".jmap.json"))) {
  accepts(file, JSON.parse(readFileSync(join(dir, file), "utf8")))
}

// The mistakes the schema is there to catch, each one a thing jmapc reports at
// build time and an editor can report while it is being typed.
refuses("a misspelled method", { methodCalls: [["Email/gett", {}, "c0"]] })
refuses("a misspelled argument", { methodCalls: [["Email/query", { fitler: {} }, "c0"]] })
refuses("a misspelled filter condition", {
  methodCalls: [["Email/query", { filter: { hasAttachmnt: true } }, "c0"]],
})
refuses("a misspelled condition inside an operator", {
  methodCalls: [["Email/query", { filter: { operator: "AND", conditions: [{ hasAttachmnt: true }] } }, "c0"]],
})
refuses("an operator that is not one", {
  methodCalls: [["Email/query", { filter: { operator: "MAYBE", conditions: [] } }, "c0"]],
})
refuses("a limit written as words", { methodCalls: [["Email/query", { limit: "ten" }, "c0"]] })
refuses("a property the type does not have", {
  methodCalls: [["Email/get", { ids: null, properties: ["subjekt"] }, "c0"]],
})
refuses("a sort by something unsortable", {
  methodCalls: [["Email/query", { sort: [{ property: "recievedAt" }] }, "c0"]],
})
refuses("a UTCDate that is only a date", {
  methodCalls: [["Email/query", { filter: { before: "2026-09-04" } }, "c0"]],
})
refuses("a member of jmapc's own that is misspelled", {
  methodCalls: [["Core/echo", {}, "c0"]],
  _retruns: "c0",
})
refuses("a call that is not a triple", { methodCalls: [["Core/echo", {}]] })
refuses("a query making no calls", { methodCalls: [] })
refuses("an argument left out from inside another value", {
  methodCalls: [["Email/query", { filter: { inMailbox: "{{mailboxId?}}" } }, "c0"]],
})

// And the things that look like mistakes and are not.
accepts("a parameter anywhere a value goes", {
  methodCalls: [["Email/query", { limit: "{{limit}}", filter: { inMailbox: "{{mailboxId}}" } }, "c0"]],
})
accepts("an argument the caller may leave out", {
  methodCalls: [["Email/changes", { sinceState: "{{state}}", maxChanges: "{{maxChanges?}}" }, "c0"]],
})
accepts("a property naming a header field", {
  methodCalls: [["Email/get", { ids: null, properties: ["header:List-Id:asText"] }, "c0"]],
})
accepts("a back reference in place of an argument", {
  methodCalls: [["Email/get", { "#ids": { resultOf: "s", name: "Email/query", path: "/ids" } }, "c0"]],
})
accepts("a comment on a call", { methodCalls: [["Email/get", { _comment: "why", ids: null }, "c0"]] })
accepts("a creation id where an id goes", {
  methodCalls: [["EmailSubmission/set", { create: { send: { emailId: "#draft" } } }, "c0"]],
})
accepts("the member a comparator adds", {
  methodCalls: [["Email/query", { sort: [{ property: "hasKeyword", keyword: "$seen" }] }, "c0"]],
})
accepts("a query naming the schema it is written against", {
  $schema: "../jmapc.schema.json",
  methodCalls: [["Core/echo", {}, "c0"]],
})

if (failures.length > 0) {
  throw new Error(`${failures.length} checks failed:\n  ${failures.join("\n  ")}`)
}
console.log(`the schema accepts every query in ${dir} and refuses the mistakes it claims to catch`)
