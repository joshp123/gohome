#!/usr/bin/env python3
import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import webbrowser

DEVICE_AUTH_BASE = "https://login.tado.com"
PASSWORD_AUTH_BASE = "https://auth.tado.com"
API_BASE = "https://my.tado.com/api/v2"
DEFAULT_CLIENT_ID = "1bb50063-6b0c-4d11-bd99-387f4a91cc46"


def post_form(url: str, data: dict, timeout: int = 10):
    encoded = urllib.parse.urlencode(data).encode()
    req = urllib.request.Request(url, data=encoded, headers={"Content-Type": "application/x-www-form-urlencoded"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode()
            return resp.status, json.loads(body)
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, {"error": body}


def get_json(url: str, access_token: str, timeout: int = 10):
    req = urllib.request.Request(url, headers={"Authorization": f"Bearer {access_token}"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode())


def device_authorize(auth_base: str, client_id: str, scope: str):
    status, payload = post_form(f"{auth_base}/oauth2/device_authorize", {
        "client_id": client_id,
        "scope": scope,
    })
    if status >= 300:
        raise RuntimeError(f"device authorization failed ({status}): {payload}")
    return payload


def poll_for_token(auth_base: str, client_id: str, device_code: str, interval: int, timeout_s: int):
    start = time.time()
    while time.time() - start < timeout_s:
        status, payload = post_form(f"{auth_base}/oauth2/token", {
            "client_id": client_id,
            "device_code": device_code,
            "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
        })
        if status == 200:
            return payload
        if payload.get("error") == "authorization_pending":
            time.sleep(interval)
            continue
        if payload.get("error") == "slow_down":
            time.sleep(interval + 2)
            continue
        raise RuntimeError(f"token poll failed ({status}): {payload}")
    raise RuntimeError("authorization timeout")


def password_token(client_id: str, client_secret: str, username: str, password: str, scope: str, auth_base: str):
    payload = {
        "grant_type": "password",
        "username": username,
        "password": password,
        "client_id": client_id,
        "scope": scope,
    }
    if client_secret:
        payload["client_secret"] = client_secret
    status, data = post_form(f"{auth_base}/oauth2/token", payload)
    if status >= 300:
        raise RuntimeError(f"password grant failed ({status}): {data}")
    return data


def read_password_file(path: str):
    with open(path, encoding="utf-8") as f:
        lines = [line.strip() for line in f.readlines() if line.strip()]
    if len(lines) < 2:
        raise RuntimeError("password file must contain username on line 1 and password on line 2")
    return lines[0], lines[1]


def main():
    parser = argparse.ArgumentParser(description="Tado OAuth helper (device or password grant)")
    parser.add_argument("--mode", choices=["device", "password"], default="device")
    parser.add_argument("--client-id", default=DEFAULT_CLIENT_ID)
    parser.add_argument("--client-secret", default="")
    parser.add_argument("--scope", default="offline_access")
    parser.add_argument("--out", default="/tmp/tado-refresh.json")
    parser.add_argument("--no-open", action="store_true")
    parser.add_argument("--auth-base", default=DEVICE_AUTH_BASE)
    parser.add_argument("--username", default="")
    parser.add_argument("--password", default="")
    parser.add_argument("--password-file", default="/tmp/tado.txt")
    args = parser.parse_args()

    if args.mode == "password":
        if args.auth_base == DEVICE_AUTH_BASE:
            args.auth_base = PASSWORD_AUTH_BASE
        if not args.username or not args.password:
            args.username, args.password = read_password_file(args.password_file)
        token = password_token(args.client_id, args.client_secret, args.username, args.password, args.scope, args.auth_base)
        refresh_token = token.get("refresh_token")
        access_token = token.get("access_token")
    else:
        auth = device_authorize(args.auth_base, args.client_id, args.scope)
        url = auth.get("verification_uri_complete") or auth.get("verification_uri")
        user_code = auth.get("user_code")
        interval = int(auth.get("interval", 5))

        print("Open this URL to authorize Tado:")
        print(url)
        print("")
        if user_code:
            print(f"User code: {user_code}")
        print("")

        if url and not args.no_open:
            try:
                webbrowser.open(url)
            except Exception:
                pass

        token = poll_for_token(args.auth_base, args.client_id, auth["device_code"], interval, int(auth.get("expires_in", 300)))
        refresh_token = token.get("refresh_token")
        access_token = token.get("access_token")

    if not refresh_token:
        raise RuntimeError("no refresh_token returned")

    payload = {
        "client_id": args.client_id,
        "client_secret": "",
        "refresh_token": refresh_token,
        "scope": args.scope,
    }

    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2)
    print(f"Wrote refresh token JSON to {args.out}")

    if access_token:
        try:
            me = get_json(f"{API_BASE}/me", access_token)
            homes = me.get("homes", [])
            if homes:
                ids = ", ".join(str(h.get("id")) for h in homes if h.get("id") is not None)
                print(f"Home IDs: {ids}")
            else:
                print("No homes returned from /me")
        except Exception as exc:
            print(f"Warning: failed to query /me: {exc}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        sys.exit(1)
