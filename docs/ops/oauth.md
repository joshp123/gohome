# OAuth Ops Notes

## Refresh Tokens (Rotation)
- Many providers rotate refresh tokens on use.
- Always persist the rotated token immediately.
- A revoked refresh token can coexist with a still-valid access token (services may look healthy until access expiry).

## Credentials Files
Use a **writable** state file for long-running services. JSON keys must be snake_case.

```json
{
  "provider": {
    "client_id": "...",
    "client_secret": "...",
    "refresh_token": "...",
    "scope": "..."
  }
}
```

## Agent-friendly flow
- `gohome oauth device --provider <id> --json` emits a machine-readable payload (verification URL + temp state path).
- A temp state file is written under `/tmp/gohome-oauth-<provider>-<timestamp>.json` for recovery.
- If persistence fails, rerun: `gohome oauth persist --provider <id> --state /tmp/...`.
- Add `--cleanup` to delete the temp file after a successful persist.
- Use `--persist-agenix` to write the bootstrap secret into the nix-secrets repo (defaults to `~/code/nix/nix-secrets`).

## Quick Validation (no secrets printed)
```
# MD5 of refresh token (CR/LF trimmed)
tr -d '\r\n' < /path/to/refresh.txt | md5

# Access token test (if you have one)
curl -sS -H "Authorization: Bearer $ACCESS" https://api.example.com/endpoint | jq 'length'
```
