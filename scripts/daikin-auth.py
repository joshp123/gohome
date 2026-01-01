#!/usr/bin/env python3
import argparse
import json
import sys
import threading
import time
import urllib.parse
import urllib.request
import webbrowser
from http.server import BaseHTTPRequestHandler, HTTPServer

DEFAULT_AUTHORIZE = "https://idp.onecta.daikineurope.com/v1/oidc/authorize"
DEFAULT_TOKEN = "https://idp.onecta.daikineurope.com/v1/oidc/token"
DEFAULT_SCOPE = "openid onecta:basic.integration"
DEFAULT_REDIRECT = "http://127.0.0.1:8765/callback"


class CallbackHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        params = urllib.parse.parse_qs(parsed.query)
        self.server.auth_code = params.get("code", [""])[0]
        self.server.auth_state = params.get("state", [""])[0]
        self.server.auth_error = params.get("error", [""])[0]

        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()

        if self.server.auth_error:
            self.wfile.write(f"Authorization failed: {self.server.auth_error}\n".encode())
        elif self.server.auth_code:
            self.wfile.write(b"Authorization received. You can close this window.\n")
        else:
            self.wfile.write(b"No authorization code received.\n")

    def log_message(self, _format, *_args):
        return


def start_server(host: str, port: int):
    server = HTTPServer((host, port), CallbackHandler)
    server.auth_code = ""
    server.auth_state = ""
    server.auth_error = ""
    return server


def post_form(url: str, data: dict, timeout: int = 15):
    encoded = urllib.parse.urlencode(data).encode()
    req = urllib.request.Request(url, data=encoded, headers={"Content-Type": "application/x-www-form-urlencoded"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read().decode()
        return resp.status, json.loads(body)


def main():
    parser = argparse.ArgumentParser(description="Daikin Onecta OAuth helper (authorization code flow)")
    parser.add_argument("--client-id", required=True)
    parser.add_argument("--client-secret", default="")
    parser.add_argument("--scope", default=DEFAULT_SCOPE)
    parser.add_argument("--authorize-url", default=DEFAULT_AUTHORIZE)
    parser.add_argument("--token-url", default=DEFAULT_TOKEN)
    parser.add_argument("--redirect-uri", default=DEFAULT_REDIRECT)
    parser.add_argument("--out", default="/tmp/daikin-onecta-refresh.json")
    parser.add_argument("--no-open", action="store_true")
    parser.add_argument("--timeout", type=int, default=300)
    args = parser.parse_args()

    parsed = urllib.parse.urlparse(args.redirect_uri)
    if parsed.scheme not in ("http", "https"):
        raise RuntimeError("redirect URI must be http or https")
    if not parsed.hostname or not parsed.port:
        raise RuntimeError("redirect URI must include host and port (e.g. http://127.0.0.1:8765/callback)")

    state = str(int(time.time()))
    query = {
        "response_type": "code",
        "client_id": args.client_id,
        "redirect_uri": args.redirect_uri,
        "scope": args.scope,
        "state": state,
    }
    authorize_url = args.authorize_url + "?" + urllib.parse.urlencode(query)

    server = start_server(parsed.hostname, parsed.port)
    thread = threading.Thread(target=server.handle_request, daemon=True)
    thread.start()

    print("Open this URL to authorize Daikin Onecta:")
    print(authorize_url)
    print("")
    print(f"Redirect URI: {args.redirect_uri}")
    print("Ensure this redirect URI is configured in the Daikin Developer Portal app.")

    if not args.no_open:
        try:
            webbrowser.open(authorize_url)
        except Exception:
            pass

    start = time.time()
    while time.time() - start < args.timeout:
        if server.auth_error:
            raise RuntimeError(f"authorization error: {server.auth_error}")
        if server.auth_code:
            break
        time.sleep(0.2)

    if not server.auth_code:
        raise RuntimeError("timed out waiting for authorization code")
    if server.auth_state and server.auth_state != state:
        raise RuntimeError("state mismatch; aborting")

    payload = {
        "grant_type": "authorization_code",
        "code": server.auth_code,
        "redirect_uri": args.redirect_uri,
        "client_id": args.client_id,
    }
    if args.client_secret:
        payload["client_secret"] = args.client_secret

    status, token = post_form(args.token_url, payload)
    if status >= 300:
        raise RuntimeError(f"token exchange failed ({status}): {token}")

    refresh_token = token.get("refresh_token")
    if not refresh_token:
        raise RuntimeError("no refresh_token returned; check scope/redirect URI")

    output = {
        "daikin_onecta": {
            "client_id": args.client_id,
            "client_secret": args.client_secret,
            "refresh_token": refresh_token,
            "scope": args.scope,
        }
    }

    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(output, f, indent=2)
    print(f"Wrote refresh token JSON to {args.out}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        sys.exit(1)
