#!/usr/bin/env python3
"""
FlexiBee Client — Supports all three communication channels.

Channels:
  1. "session" (default) — Web session via /v2/ API path. No API license needed.
     Auth: form POST → authSessionId cookie → x-authSessionId header on /v2/c/...
  2. "basic" — REST API via /c/ path. Requires API license + api_access_enabled.
     Auth: HTTP Basic Auth with ?auth=http parameter.
  3. "auto" — Try session first; if /v2/ returns 402, fall back to basic.

Usage:
  # Session mode (no API license required)
  with FlexiBeeClient("https://localhost:5434", "test", "test123") as client:
      invoices = client.query("faktura-vydana", limit=10)

  # Basic auth mode (API license required)
  with FlexiBeeClient("https://host:5434", "api", "pass", auth_mode="basic") as client:
      invoices = client.query("faktura-vydana", limit=10)

Protocol details: see docs/protocol-research.md
"""

import json
import sys
from typing import Any, Optional

import requests
import urllib3

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


class FlexiBeeClient:
    """FlexiBee client supporting session (v2), basic auth (v1), and auto modes."""

    def __init__(
        self,
        base_url: str,
        username: str,
        password: str,
        company: Optional[str] = None,
        verify_ssl: bool = False,
        auth_mode: str = "session",  # "session", "basic", or "auto"
    ):
        self.base_url = base_url.rstrip("/")
        self.username = username
        self.password = password
        self.company = company
        self.verify_ssl = verify_ssl
        self.auth_mode = auth_mode

        self.auth_session_id: Optional[str] = None
        self.csrf_token: Optional[str] = None
        self._effective_mode: str = auth_mode  # resolved at login time

        self.session = requests.Session()
        self.session.verify = verify_ssl

    def __enter__(self):
        self.login()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.logout()
        return False

    # ── Path helpers ─────────────────────────────────────────────────

    def _data_prefix(self) -> str:
        """Return the URL prefix for data endpoints based on auth mode."""
        if self._effective_mode == "basic":
            return "/c"
        return "/v2/c"

    def _evidence_url(self, evidence: str, identifier: Any = None,
                      company: Optional[str] = None) -> str:
        co = company or self.company
        base = f"{self._data_prefix()}/{co}/{evidence}"
        if identifier is not None:
            base += f"/{identifier}"
        return base

    # ── Authentication ───────────────────────────────────────────────

    def login(self) -> dict:
        """Authenticate based on the configured auth_mode."""
        if self.auth_mode == "basic":
            return self._login_basic()
        elif self.auth_mode == "session":
            return self._login_session()
        else:  # auto
            try:
                result = self._login_session()
                self._effective_mode = "session"
                return result
            except RuntimeError:
                result = self._login_basic()
                self._effective_mode = "basic"
                return result

    def _login_session(self) -> dict:
        """Authenticate via web form login (session mode)."""
        resp = self.session.post(
            f"{self.base_url}/login-logout/login",
            data={"username": self.username, "password": self.password},
            allow_redirects=False,
        )
        if resp.status_code == 303:
            location = resp.headers.get("Location", "")
            if "error=" in location:
                raise RuntimeError("Login failed: wrong credentials")
        elif resp.status_code >= 400:
            raise RuntimeError(f"Login failed: HTTP {resp.status_code}")

        self.auth_session_id = self.session.cookies.get("authSessionId")
        self.csrf_token = self.session.cookies.get("CSRF-Token")
        if not self.auth_session_id:
            raise RuntimeError("Login failed: no authSessionId cookie")

        self._effective_mode = "session"
        self._auto_discover_company()
        return {"authSessionId": self.auth_session_id, "company": self.company,
                "mode": "session"}

    def _login_basic(self) -> dict:
        """Set up Basic Auth credentials (no login request needed)."""
        self.session.auth = (self.username, self.password)
        self._effective_mode = "basic"
        self._auto_discover_company()
        return {"company": self.company, "mode": "basic"}

    def _auto_discover_company(self):
        if not self.company:
            try:
                companies = self.list_companies()
                if companies:
                    key = "dbNazev" if "dbNazev" in companies[0] else "dbNazev"
                    self.company = companies[0].get(key)
            except Exception:
                pass

    def logout(self) -> None:
        """End the session (session mode) or clear auth (basic mode)."""
        if self._effective_mode == "session" and self.auth_session_id:
            try:
                requests.get(
                    f"{self.base_url}/login-logout/logout",
                    headers={"x-authSessionId": self.auth_session_id},
                    verify=self.verify_ssl, allow_redirects=False, timeout=5,
                )
            except Exception:
                pass
        self.auth_session_id = None
        self.csrf_token = None
        self.session.auth = None
        self.session.cookies.clear()

    def keepalive(self) -> dict:
        """Send session keepalive."""
        return self._request("GET", "/login-logout/session-keep-alive.js")

    # ── Discovery ────────────────────────────────────────────────────

    def list_companies(self) -> list[dict]:
        """List all companies on the server."""
        prefix = "/v2/c" if self._effective_mode == "session" else "/c"
        data = self._request("GET", f"{prefix}.json")
        companies = data.get("companies", {}).get("company", [])
        if isinstance(companies, dict):
            companies = [companies]
        return companies

    def list_evidences(self, company: Optional[str] = None) -> list[dict]:
        """List all evidence types (record collections) in a company."""
        co = company or self.company
        data = self._request("GET", f"{self._data_prefix()}/{co}/evidence-list.json")
        evidences = data.get("evidences", {}).get("evidence", [])
        if isinstance(evidences, dict):
            evidences = [evidences]
        return evidences

    def describe_evidence(self, evidence: str, section: str = "properties",
                          company: Optional[str] = None) -> dict:
        """Get evidence metadata: properties, relations, reports, actions."""
        url = f"{self._evidence_url(evidence, company=company)}/{section}.json"
        return self._request("GET", url)

    def get_metadata(self, company: Optional[str] = None) -> dict:
        """Get full metadata for all evidences (~1MB). Use sparingly."""
        co = company or self.company
        return self._request("GET", f"{self._data_prefix()}/{co}/metadata.json")

    # ── Read ─────────────────────────────────────────────────────────

    def query(
        self,
        evidence: str,
        filter: Optional[str] = None,
        fields: Optional[list[str]] = None,
        sort: Optional[str] = None,
        limit: int = 20,
        start: int = 0,
        add_row_count: bool = False,
        detail: Optional[str] = None,
        company: Optional[str] = None,
    ) -> list[dict]:
        """Query records from an evidence with filtering, paging, field selection."""
        params: dict[str, str] = {"limit": str(limit), "start": str(start)}
        if filter:
            params["filter"] = filter
        if fields:
            params["detail"] = "custom:" + ",".join(fields)
        elif detail:
            params["detail"] = detail
        if sort:
            params["sort"] = sort
        if add_row_count:
            params["add-row-count"] = "true"

        url = f"{self._evidence_url(evidence, company=company)}.json"
        data = self._request("GET", url, params=params)
        records = data.get("winstrom", {}).get(evidence, [])
        if isinstance(records, dict):
            records = [records]
        return records

    def get_record(self, evidence: str, identifier: Any,
                   fields: Optional[list[str]] = None,
                   detail: Optional[str] = None,
                   company: Optional[str] = None) -> dict:
        """Get a single record by ID or code."""
        params = {}
        if fields:
            params["detail"] = "custom:" + ",".join(fields)
        elif detail:
            params["detail"] = detail
        url = f"{self._evidence_url(evidence, identifier, company)}.json"
        data = self._request("GET", url, params=params or None)
        records = data.get("winstrom", {}).get(evidence, [])
        if isinstance(records, list) and records:
            return records[0]
        return records if isinstance(records, dict) else {}

    def get_related(self, evidence: str, identifier: Any, relation: str,
                    company: Optional[str] = None) -> list[dict]:
        """Get related records (sub-evidence, e.g. invoice line items)."""
        url = f"{self._evidence_url(evidence, identifier, company)}/{relation}.json"
        data = self._request("GET", url)
        records = data.get("winstrom", {}).get(relation, [])
        if isinstance(records, dict):
            records = [records]
        return records

    def get_sum(self, evidence: str, filter: Optional[str] = None,
                company: Optional[str] = None) -> dict:
        """Get aggregated document totals ($sum endpoint)."""
        params = {"filter": filter} if filter else None
        url = f"{self._evidence_url(evidence, company=company)}/$sum.json"
        return self._request("GET", url, params=params)

    def get_report(self, evidence: str, identifier: Any, format: str = "pdf",
                   report_name: Optional[str] = None,
                   company: Optional[str] = None) -> bytes:
        """Download a report (PDF, XLSX, etc.) for a record."""
        params = {}
        if report_name:
            params["report-name"] = report_name
        url = f"{self._evidence_url(evidence, identifier, company)}.{format}"
        resp = self._raw_request("GET", url, params=params or None)
        return resp.content

    def get_attachment_list(self, evidence: str, identifier: Any,
                           company: Optional[str] = None) -> list[dict]:
        """List attachments for a record."""
        return self.get_related(evidence, identifier, "prilohy", company)

    def get_attachment_content(self, evidence: str, identifier: Any,
                              attachment_id: Any,
                              company: Optional[str] = None) -> bytes:
        """Download attachment content."""
        url = (f"{self._evidence_url(evidence, identifier, company)}"
               f"/prilohy/{attachment_id}/content")
        resp = self._raw_request("GET", url)
        return resp.content

    def export(self, evidence: str, format: str = "csv",
               filter: Optional[str] = None, limit: int = 0,
               company: Optional[str] = None) -> str:
        """Export records in CSV, XML, or other text format."""
        params: dict[str, str] = {"limit": str(limit)}
        if filter:
            params["filter"] = filter
        url = f"{self._evidence_url(evidence, company=company)}.{format}"
        resp = self._raw_request("GET", url, params=params)
        return resp.text

    # ── Write ────────────────────────────────────────────────────────

    def create(self, evidence: str, record: dict, dry_run: bool = False,
               company: Optional[str] = None) -> dict:
        """Create a new record. Pass a dict or list of dicts for batch create."""
        payload = {"winstrom": {evidence: record}}
        params = {"dry-run": "true"} if dry_run else None
        url = f"{self._evidence_url(evidence, company=company)}.json"
        return self._request("PUT", url, json_body=payload, params=params)

    def update(self, evidence: str, identifier: Any, record: dict,
               dry_run: bool = False, company: Optional[str] = None) -> dict:
        """Update an existing record."""
        record["id"] = str(identifier)
        payload = {"winstrom": {evidence: record}}
        params = {"dry-run": "true"} if dry_run else None
        url = f"{self._evidence_url(evidence, identifier, company)}.json"
        return self._request("PUT", url, json_body=payload, params=params)

    def delete(self, evidence: str, identifier: Any,
               company: Optional[str] = None) -> dict:
        """Delete a record."""
        url = f"{self._evidence_url(evidence, identifier, company)}.json"
        return self._request("DELETE", url)

    def storno(self, evidence: str, identifier: Any,
               company: Optional[str] = None) -> dict:
        """Storno (reverse) a document record."""
        url = f"{self._evidence_url(evidence, identifier, company)}.json"
        return self._request("POST", url,
                             json_body={"winstrom": {evidence: {
                                 "id": str(identifier), "action": "storno"}}})

    # ── Server / License / Users ─────────────────────────────────────

    def get_status(self) -> dict:
        """Get server status (version, sessions, memory)."""
        return self._request("GET", "/status.json")

    def get_license(self) -> dict:
        """Get license information."""
        prefix = "/v2" if self._effective_mode == "session" else ""
        return self._request("GET", f"{prefix}/license.json")

    def list_users(self) -> list[dict]:
        """List server users."""
        prefix = "/v2" if self._effective_mode == "session" else ""
        data = self._request("GET", f"{prefix}/u.json")
        users = data.get("winstrom", {}).get("uzivatel", [])
        if isinstance(users, dict):
            users = [users]
        return users

    # ── Changes API ──────────────────────────────────────────────────

    def get_changes(self, start: int = 0, limit: int = 100,
                    company: Optional[str] = None) -> dict:
        """Get change log (requires changelog enabled on the company)."""
        co = company or self.company
        return self._request("GET", f"{self._data_prefix()}/{co}/changes.json",
                             params={"start": str(start), "limit": str(limit)})

    # ── Internal ─────────────────────────────────────────────────────

    def _build_headers(self, method: str) -> dict[str, str]:
        headers: dict[str, str] = {"Accept": "application/json"}
        if self._effective_mode == "session" and self.auth_session_id:
            headers["x-authSessionId"] = self.auth_session_id
        return headers

    def _build_params(self, method: str,
                      params: Optional[dict] = None) -> Optional[dict]:
        """Add auth and CSRF params as needed."""
        if params is None:
            params = {}
        if self._effective_mode == "basic":
            params["auth"] = "http"
        if self._effective_mode == "session" and self.csrf_token:
            if method in ("PUT", "POST", "DELETE"):
                params["token"] = self.csrf_token
        return params if params else None

    def _request(self, method: str, path: str,
                 json_body: Optional[dict] = None,
                 params: Optional[dict] = None) -> dict:
        """Make an authenticated JSON request."""
        resp = self._raw_request(method, path, json_body=json_body, params=params)

        if resp.status_code >= 400:
            try:
                return resp.json()
            except (json.JSONDecodeError, ValueError):
                raise RuntimeError(f"HTTP {resp.status_code}: {resp.text[:500]}")

        if resp.text:
            try:
                return resp.json()
            except (json.JSONDecodeError, ValueError):
                return {"_raw": resp.text[:1000]}
        return {}

    def _raw_request(self, method: str, path: str,
                     json_body: Optional[dict] = None,
                     params: Optional[dict] = None) -> requests.Response:
        """Make an authenticated raw request (returns Response object)."""
        url = f"{self.base_url}{path}"
        headers = self._build_headers(method)
        params = self._build_params(method, params)

        resp = self.session.request(
            method, url, headers=headers, json=json_body,
            params=params, allow_redirects=False,
        )

        # Handle redirects
        if resp.status_code in (301, 302, 303, 307, 308):
            location = resp.headers.get("Location", "")
            if "remote-logout" in location or "login-form" in location:
                raise RuntimeError(
                    f"Session expired or invalid. Redirected to: {location}")
            if location.startswith("/"):
                location = f"{self.base_url}{location}"
            resp = self.session.request(
                "GET" if resp.status_code == 303 else method,
                location, headers=headers, allow_redirects=False,
            )

        return resp


# ── CLI ──────────────────────────────────────────────────────────────

def main():
    import argparse
    p = argparse.ArgumentParser(description="FlexiBee client (session/basic/auto)")
    p.add_argument("--url", default="https://localhost:5434")
    p.add_argument("--user", default="test")
    p.add_argument("--password", default="test123")
    p.add_argument("--company", default=None)
    p.add_argument("--auth-mode", default="session",
                   choices=["session", "basic", "auto"])
    p.add_argument("action",
                   choices=["info", "evidences", "query", "get", "create",
                            "update", "delete", "status", "license",
                            "users", "sum", "export", "report"])
    p.add_argument("-e", "--evidence", help="Evidence name")
    p.add_argument("--id", help="Record ID")
    p.add_argument("--fields", help="Comma-separated fields")
    p.add_argument("--filter", help="Filter expression")
    p.add_argument("--limit", type=int, default=20)
    p.add_argument("--data", help="JSON data for create/update")
    p.add_argument("--format", default="csv", help="Export format")
    p.add_argument("-o", "--output", help="Output file for report/export")
    args = p.parse_args()

    client = FlexiBeeClient(args.url, args.user, args.password,
                            args.company, auth_mode=args.auth_mode)
    info = client.login()
    print(f"Mode: {info.get('mode','?')}, Company: {info.get('company','?')}",
          file=sys.stderr)
    try:
        if args.action == "info":
            print(json.dumps(client.list_companies(), indent=2, ensure_ascii=False))

        elif args.action == "evidences":
            for ev in client.list_evidences():
                print(f"  {ev.get('evidencePath',''):40s} {ev.get('evidenceName','')}")

        elif args.action == "query":
            if not args.evidence: p.error("--evidence required")
            fields = args.fields.split(",") if args.fields else None
            recs = client.query(args.evidence, filter=args.filter,
                                fields=fields, limit=args.limit)
            print(json.dumps(recs, indent=2, ensure_ascii=False))

        elif args.action == "get":
            if not args.evidence or not args.id: p.error("--evidence and --id required")
            fields = args.fields.split(",") if args.fields else None
            rec = client.get_record(args.evidence, args.id, fields=fields)
            print(json.dumps(rec, indent=2, ensure_ascii=False))

        elif args.action == "create":
            if not args.evidence or not args.data: p.error("--evidence and --data required")
            print(json.dumps(client.create(args.evidence, json.loads(args.data)),
                             indent=2, ensure_ascii=False))

        elif args.action == "update":
            if not args.evidence or not args.id or not args.data:
                p.error("--evidence, --id, and --data required")
            print(json.dumps(client.update(args.evidence, args.id, json.loads(args.data)),
                             indent=2, ensure_ascii=False))

        elif args.action == "delete":
            if not args.evidence or not args.id: p.error("--evidence and --id required")
            print(json.dumps(client.delete(args.evidence, args.id),
                             indent=2, ensure_ascii=False))

        elif args.action == "status":
            print(json.dumps(client.get_status(), indent=2, ensure_ascii=False))

        elif args.action == "license":
            print(json.dumps(client.get_license(), indent=2, ensure_ascii=False))

        elif args.action == "users":
            print(json.dumps(client.list_users(), indent=2, ensure_ascii=False))

        elif args.action == "sum":
            if not args.evidence: p.error("--evidence required")
            print(json.dumps(client.get_sum(args.evidence, filter=args.filter),
                             indent=2, ensure_ascii=False))

        elif args.action == "export":
            if not args.evidence: p.error("--evidence required")
            content = client.export(args.evidence, format=args.format,
                                    filter=args.filter, limit=args.limit)
            if args.output:
                with open(args.output, "w") as f:
                    f.write(content)
                print(f"Exported to {args.output}", file=sys.stderr)
            else:
                print(content)

        elif args.action == "report":
            if not args.evidence or not args.id: p.error("--evidence and --id required")
            data = client.get_report(args.evidence, args.id, format=args.format)
            out = args.output or f"{args.evidence}_{args.id}.{args.format}"
            with open(out, "wb") as f:
                f.write(data)
            print(f"Report saved to {out} ({len(data)} bytes)", file=sys.stderr)

    finally:
        client.logout()


if __name__ == "__main__":
    main()
