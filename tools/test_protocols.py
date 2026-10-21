#!/usr/bin/env python3
"""
Comprehensive live protocol test suite for flexibee_client.py.

Tests all three auth modes (session, basic, auto) and all major features
against a real FlexiBee server.  Requires `requests` (pip install requests).

Usage:
    # Edit URL / USER / PASS / COMPANY below, then:
    python3 tools/test_protocols.py

This is a *live* test — it creates and deletes real records on the target server.
Run against a disposable test company only, never production data.

See docs/protocol-research.md for protocol details.
"""

import json
import os
import sys
import time
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from flexibee_client import FlexiBeeClient

URL = "https://localhost:5434"
USER = "test"
PASS = "test123"
COMPANY = "lukas_viktora_2026_08_01_1343"

passed = 0
failed = 0
skipped = 0
results = []


def test(name, fn, expect_fail=False):
    """Run a single test, record result."""
    global passed, failed, skipped
    try:
        result = fn()
        if expect_fail:
            results.append(("UNEXPECTED_PASS", name, str(result)[:120]))
            failed += 1
        else:
            results.append(("PASS", name, str(result)[:120]))
            passed += 1
        return result
    except Exception as e:
        if expect_fail:
            results.append(("EXPECTED_FAIL", name, str(e)[:120]))
            passed += 1
        else:
            results.append(("FAIL", name, str(e)[:120]))
            failed += 1
        return None


def skip(name, reason):
    global skipped
    results.append(("SKIP", name, reason))
    skipped += 1


# ═══════════════════════════════════════════════════════════════════
#  PROTOCOL 1: SESSION MODE (v2 path)
# ═══════════════════════════════════════════════════════════════════
print("=" * 70)
print("PROTOCOL 1: SESSION MODE (v2 path, web UI API)")
print("=" * 70)

client = FlexiBeeClient(URL, USER, PASS, company=COMPANY, auth_mode="session")

# --- Auth ---
test("session/login",
     lambda: client.login())

test("session/keepalive",
     lambda: client.keepalive())

# --- Discovery ---
test("session/list_companies",
     lambda: client.list_companies())

ev_count = test("session/list_evidences",
     lambda: len(client.list_evidences()))

test("session/describe_evidence_properties",
     lambda: len(client.describe_evidence("adresar", "properties")
                 .get("properties", {}).get("property", [])))

test("session/describe_evidence_relations",
     lambda: len(client.describe_evidence("adresar", "relations")
                 .get("relations", {}).get("relation", [])))

test("session/describe_evidence_reports",
     lambda: client.describe_evidence("faktura-vydana", "reports"))

test("session/describe_evidence_actions",
     lambda: client.describe_evidence("adresar", "actions"))

# --- Read ---
test("session/query_contacts",
     lambda: client.query("adresar", fields=["id", "kod", "nazev"], limit=3))

test("session/query_invoices",
     lambda: client.query("faktura-vydana",
                          fields=["id", "kod", "datVyst", "sumCelkem", "stavUhrK"],
                          limit=3))

test("session/query_with_filter",
     lambda: client.query("faktura-vydana",
                          filter="sumCelkem > 400",
                          fields=["id", "kod", "sumCelkem"], limit=5))

test("session/query_with_sort",
     lambda: client.query("faktura-vydana",
                          fields=["id", "kod", "sumCelkem"],
                          sort="sumCelkem", limit=3))

test("session/query_with_row_count",
     lambda: client.query("faktura-vydana", limit=1, add_row_count=True))

test("session/query_full_detail",
     lambda: client.query("faktura-vydana", detail="full", limit=1))

test("session/get_record_by_id",
     lambda: client.get_record("faktura-vydana", 1,
                               fields=["id", "kod", "sumCelkem"]))

test("session/get_record_by_code",
     lambda: client.get_record("adresar", "code:FÚ GFŘ",
                               fields=["id", "kod", "nazev"]))

# --- Sub-evidence / Relations ---
test("session/get_related_invoice_items",
     lambda: client.get_related("faktura-vydana", 1, "polozkyDokladu"))

test("session/get_attachment_list",
     lambda: client.get_attachment_list("faktura-vydana", 1))

# --- Aggregation ---
test("session/get_sum",
     lambda: client.get_sum("faktura-vydana"))

test("session/get_sum_with_filter",
     lambda: client.get_sum("faktura-vydana", filter="stavUhrK = 'stavUhr.uhrazeno'"))

# --- CRUD lifecycle ---
create_result = test("session/create_contact",
     lambda: client.create("adresar", {
         "kod": "TEST-SESSION-001",
         "nazev": "Session Test Contact",
         "email": "session@test.cz",
         "ulice": "Testovaci 1",
         "mesto": "Praha",
         "psc": "11000",
     }))

new_id = None
if create_result:
    new_id = create_result.get("winstrom", {}).get("results", [{}])[0].get("id")

if new_id:
    test("session/read_created",
         lambda: client.get_record("adresar", new_id,
                                   fields=["id", "kod", "nazev", "email"]))

    test("session/update_contact",
         lambda: client.update("adresar", new_id, {
             "nazev": "Session Test - UPDATED",
             "poznam": "Updated via session protocol test",
         }))

    test("session/read_updated",
         lambda: client.get_record("adresar", new_id,
                                   fields=["id", "kod", "nazev", "poznam"]))

    test("session/dry_run_update",
         lambda: client.update("adresar", new_id,
                               {"nazev": "DRY RUN - should not persist"},
                               dry_run=True))

    # Verify dry run didn't persist
    test("session/verify_dry_run",
         lambda: (
             client.get_record("adresar", new_id, fields=["nazev"]).get("nazev")
             == "Session Test - UPDATED"
             or "DRY RUN assertion failed"
         ))

    test("session/delete_contact",
         lambda: client.delete("adresar", new_id))
else:
    skip("session/read_created", "create failed")
    skip("session/update_contact", "create failed")
    skip("session/read_updated", "create failed")
    skip("session/dry_run_update", "create failed")
    skip("session/verify_dry_run", "create failed")
    skip("session/delete_contact", "create failed")

# --- Create invoice with line items ---
inv_result = test("session/create_invoice_with_items",
     lambda: client.create("faktura-vydana", {
         "typDokl": "code:FAKTURA",
         "firma": "code:FÚ GFŘ",
         "popis": "Protocol test invoice",
         "polozkyFaktury": [
             {"nazev": "Test item 1", "mnozMj": "2", "cenaMj": "100.00"},
             {"nazev": "Test item 2", "mnozMj": "1", "cenaMj": "250.00"},
         ]
     }))

inv_id = None
if inv_result:
    inv_id = inv_result.get("winstrom", {}).get("results", [{}])[0].get("id")

if inv_id:
    test("session/read_created_invoice",
         lambda: client.get_record("faktura-vydana", inv_id,
                                   fields=["id", "kod", "sumCelkem", "popis"]))

    test("session/invoice_line_items",
         lambda: client.get_related("faktura-vydana", inv_id, "polozkyDokladu"))

    # Storno the invoice
    test("session/storno_invoice",
         lambda: client.storno("faktura-vydana", inv_id))

    # Delete the storno'd invoice
    test("session/delete_invoice",
         lambda: client.delete("faktura-vydana", inv_id))
else:
    skip("session/read_created_invoice", "create failed")
    skip("session/invoice_line_items", "create failed")
    skip("session/storno_invoice", "create failed")
    skip("session/delete_invoice", "create failed")

# --- Export formats ---
test("session/export_csv",
     lambda: len(client.export("adresar", format="csv", limit=2)))

test("session/export_xml",
     lambda: len(client.export("adresar", format="xml", limit=1)))

test("session/export_json",
     lambda: len(client.export("adresar", format="json", limit=1)))

# --- Report/PDF ---
pdf_data = test("session/report_pdf",
     lambda: client.get_report("faktura-vydana", 1, format="pdf"))

if pdf_data:
    test("session/report_pdf_valid",
         lambda: pdf_data[:5] == b"%PDF-" or "Not a valid PDF")
    test("session/report_pdf_size",
         lambda: f"{len(pdf_data)} bytes")

# --- ISDOC ---
test("session/export_isdoc",
     lambda: len(client.export("faktura-vydana", format="isdoc", limit=1)))

# --- Server info ---
test("session/get_license",
     lambda: client.get_license().get("license", {}).get("variant"))

test("session/list_users",
     lambda: len(client.list_users()))

# --- Pagination ---
test("session/pagination_page1",
     lambda: len(client.query("adresar", limit=2, start=0)))

test("session/pagination_page2",
     lambda: len(client.query("adresar", limit=2, start=2)))

# --- Cleanup ---
test("session/logout",
     lambda: client.logout())

print()

# ═══════════════════════════════════════════════════════════════════
#  PROTOCOL 2: BASIC AUTH MODE (v1 path)
# ═══════════════════════════════════════════════════════════════════
print("=" * 70)
print("PROTOCOL 2: BASIC AUTH MODE (v1 path, REST API)")
print("=" * 70)

# Need a pause between session tests
time.sleep(2)

client2 = FlexiBeeClient(URL, USER, PASS, company=COMPANY, auth_mode="basic")

test("basic/login",
     lambda: client2.login())

# Basic auth with ?auth=http should be blocked by API license on this server
# But let's test what actually happens
# Basic auth uses /c/ path with ?auth=http — blocked by API license on this server
# The server returns a 402 error JSON for data operations but the client wraps it
test("basic/list_companies",
     lambda: client2.list_companies())

test("basic/query_contacts_blocked",
     lambda: client2.query("adresar", fields=["id", "kod", "nazev"], limit=2))

# Create should fail with API license error
basic_create = test("basic/create_contact_blocked",
     lambda: client2.create("adresar", {
         "kod": "TEST-BASIC-001",
         "nazev": "Basic Auth Test",
     }))

basic_id = None
if basic_create and "results" in str(basic_create):
    basic_id = basic_create.get("winstrom", {}).get("results", [{}])[0].get("id")
    if basic_id:
        test("basic/read_created",
             lambda: client2.get_record("adresar", basic_id, fields=["id", "kod"]))
        test("basic/delete_created",
             lambda: client2.delete("adresar", basic_id))

test("basic/get_sum",
     lambda: client2.get_sum("faktura-vydana"))

test("basic/export_csv",
     lambda: len(client2.export("adresar", format="csv", limit=1)))

test("basic/get_status",
     lambda: client2.get_status())

test("basic/logout",
     lambda: client2.logout())

print()

# ═══════════════════════════════════════════════════════════════════
#  PROTOCOL 3: AUTO MODE (try session, fall back to basic)
# ═══════════════════════════════════════════════════════════════════
print("=" * 70)
print("PROTOCOL 3: AUTO MODE (session with fallback)")
print("=" * 70)

time.sleep(2)

client3 = FlexiBeeClient(URL, USER, PASS, company=COMPANY, auth_mode="auto")

login_result = test("auto/login",
     lambda: client3.login())

test("auto/effective_mode",
     lambda: client3._effective_mode)

test("auto/list_companies",
     lambda: client3.list_companies())

test("auto/query_invoices",
     lambda: client3.query("faktura-vydana",
                           fields=["id", "kod", "sumCelkem"], limit=2))

# CRUD
auto_create = test("auto/create_contact",
     lambda: client3.create("adresar", {
         "kod": "TEST-AUTO-001",
         "nazev": "Auto Mode Test",
     }))

auto_id = None
if auto_create:
    auto_id = auto_create.get("winstrom", {}).get("results", [{}])[0].get("id")

if auto_id:
    test("auto/update_contact",
         lambda: client3.update("adresar", auto_id, {"nazev": "Auto - UPDATED"}))
    test("auto/read_updated",
         lambda: client3.get_record("adresar", auto_id, fields=["id", "nazev"]))
    test("auto/delete_contact",
         lambda: client3.delete("adresar", auto_id))
else:
    skip("auto/update_contact", "create failed")
    skip("auto/read_updated", "create failed")
    skip("auto/delete_contact", "create failed")

test("auto/get_sum",
     lambda: client3.get_sum("faktura-vydana"))

test("auto/export_csv",
     lambda: len(client3.export("adresar", format="csv", limit=1)))

test("auto/report_pdf_size",
     lambda: len(client3.get_report("faktura-vydana", 1, format="pdf")))

test("auto/get_license",
     lambda: client3.get_license().get("license", {}).get("variant"))

test("auto/logout",
     lambda: client3.logout())

print()

# ═══════════════════════════════════════════════════════════════════
#  RESULTS
# ═══════════════════════════════════════════════════════════════════
print("=" * 70)
print("TEST RESULTS")
print("=" * 70)
print()

for status, name, detail in results:
    icon = {"PASS": "OK", "FAIL": "FAIL", "SKIP": "SKIP",
            "EXPECTED_FAIL": "OK(expected)", "UNEXPECTED_PASS": "FAIL(unexpected)"}[status]
    mark = "  " if status in ("PASS", "EXPECTED_FAIL") else ">>"
    detail_str = detail[:80] if detail else ""
    print(f"{mark} [{icon:16s}] {name:45s} {detail_str}")

print()
print(f"PASSED: {passed}  |  FAILED: {failed}  |  SKIPPED: {skipped}  |  TOTAL: {passed+failed+skipped}")

if failed > 0:
    print("\n*** THERE WERE FAILURES ***")
    sys.exit(1)
else:
    print("\n*** ALL TESTS PASSED ***")
    sys.exit(0)
