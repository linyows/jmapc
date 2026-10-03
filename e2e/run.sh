#!/bin/sh
# Runs the end-to-end tests: starts Stalwart in a container, sets it up, and
# runs the scenarios against it. Arguments are passed to the probe run of the
# scenarios, as in `e2e/run.sh --report github-summary`.
#
# It needs docker, go and probe on the PATH. E2E_HTTP_PORT, E2E_SMTP_PORT and
# E2E_IMAP_PORT choose the ports on this side (18080, 10025 and 10993), and
# E2E_KEEP=1 leaves the container running afterwards.
set -eu

# The release the tests are written against. Stalwart moves its settings
# between minor versions, so a new one is a change to make here on purpose.
image=stalwartlabs/stalwart:v0.16@sha256:ec011be228596e37e65f41aab17deed573859614430472f7eeb42178c50d87b7

cd "$(dirname "$0")/.."

secret() {
	LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24
}

http_port=${E2E_HTTP_PORT:-18080}
smtp_port=${E2E_SMTP_PORT:-10025}
imap_port=${E2E_IMAP_PORT:-10993}
work=$(mktemp -d)

export E2E_CONTAINER=${E2E_CONTAINER:-jmapc-e2e}
export E2E_BASE_URL=http://localhost:$http_port
export E2E_SMTP_ADDR=localhost:$smtp_port
export E2E_IMAP_HOST=localhost
export E2E_IMAP_PORT=$imap_port
export E2E_ROOT=$PWD
export E2E_DRIVER=$work/driver
# Generated for the run unless given, which is what lets a run kept with
# E2E_KEEP=1 be logged into afterwards.
export E2E_RECOVERY_ADMIN=${E2E_RECOVERY_ADMIN:-admin:$(secret)}
export E2E_ALICE_PASSWORD=${E2E_ALICE_PASSWORD:-$(secret)}
export E2E_BOB_PASSWORD=${E2E_BOB_PASSWORD:-$(secret)}

cleanup() {
	if [ "${E2E_KEEP:-}" != 1 ]; then
		docker rm -f "$E2E_CONTAINER" >/dev/null 2>&1 || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT

docker rm -f "$E2E_CONTAINER" >/dev/null 2>&1 || true
# STALWART_PUBLIC_URL is what the session's URLs are built from. Without it
# they name the server's hostname, which nothing outside the container
# resolves.
docker run -d --name "$E2E_CONTAINER" \
	-p "$http_port:8080" -p "$smtp_port:25" -p "$imap_port:993" \
	-e STALWART_RECOVERY_ADMIN="$E2E_RECOVERY_ADMIN" \
	-e STALWART_PUBLIC_URL="$E2E_BASE_URL" \
	"$image" >/dev/null

probe e2e/vars.yml,e2e/setup.yml
probe "$@" e2e/vars.yml,e2e/workflow.yml
