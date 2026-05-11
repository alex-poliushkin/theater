#!/usr/bin/env python3
import copy
import datetime as dt
import json
import os
import sys
import time


PLUGIN = {"id": "smoke-plugin", "version": "0.2.0"}
PROTOCOL = {"name": "theater-jsonrpc", "major": 1, "minor": 0}
DIGEST = "sha256:b613b0132363b27a38d1e7d99980695df6e1af5929493e75845e6a38173f507c"
PREPARED = set()
SUPPORTED = {
    "inventory.smoke.echo",
    "action.smoke.echo",
    "action.smoke.secret_fail",
    "action.smoke.sleep",
    "action.smoke.validate_probe",
    "report_exporter.smoke.write",
    "state_backend.smoke.file",
    "transform.smoke.wrap",
    "matcher.smoke.equal",
}


def read_frame():
    content_length = None
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            return None
        line = line.decode("utf-8").rstrip("\r\n")
        if line == "":
            break
        name, value = line.split(":", 1)
        if name.lower() == "content-length":
            content_length = int(value.strip())
    if content_length is None:
        raise RuntimeError("missing Content-Length header")
    payload = sys.stdin.buffer.read(content_length)
    return json.loads(payload.decode("utf-8"))


def write_frame(payload):
    raw = json.dumps(payload).encode("utf-8")
    sys.stdout.buffer.write(f"Content-Length: {len(raw)}\r\n\r\n".encode("utf-8"))
    sys.stdout.buffer.write(raw)
    sys.stdout.buffer.flush()


def notify(method, params):
    write_frame({"jsonrpc": "2.0", "method": method, "params": params})


def plugin_error(message, theater_code="plugin_failed", partial=None):
    data = {"theater_code": theater_code}
    if partial is not None:
        data["partial_outputs"] = partial
    return {"code": -32001, "message": message, "data": data}


def validate_probe_shape(params):
    properties = params.get("properties") or {}
    dynamic_paths = set(params.get("dynamic_paths") or [])
    expected_paths = {"/dynamic", "/object/dynamic", "/items/1", "/items"}
    expected_keys = {"mode", "literal", "secret", "object"}
    errors = []
    if set(properties.keys()) != expected_keys:
        errors.append(f"properties keys mismatch: got {sorted(properties.keys())}")
    if dynamic_paths != expected_paths:
        errors.append(f"dynamic paths mismatch: got {sorted(dynamic_paths)}")
    if properties.get("literal") != "static":
        errors.append("literal property missing")
    if properties.get("secret") != "validate-secret":
        errors.append("secret property missing")
    if "dynamic" in properties:
        errors.append("dynamic property was resolved too early")
    if "missing" in properties or "/missing" in dynamic_paths:
        errors.append("absent property was reported as present")
    nested = properties.get("object") or {}
    if set(nested.keys()) != {"literal"}:
        errors.append(f"object keys mismatch: got {sorted(nested.keys())}")
    if nested.get("literal") != "nested-static":
        errors.append("object literal property missing")
    if "items" in properties:
        errors.append("dynamic list was resolved too early")
    return errors


def now_utc():
    return dt.datetime.now(dt.timezone.utc)


def encode_time(value):
    return value.astimezone(dt.timezone.utc).isoformat().replace("+00:00", "Z")


def parse_time(raw):
    if not raw:
        return None
    return dt.datetime.fromisoformat(raw.replace("Z", "+00:00"))


def load_store(path):
    if not path:
        raise RuntimeError("state backend path is required")
    if not os.path.exists(path):
        return {"records": {}, "pools": {}}
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict):
        raise RuntimeError("state store must be a JSON object")
    data.setdefault("records", {})
    data.setdefault("pools", {})
    return data


def save_store(path, store):
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(store, fh, indent=2, sort_keys=True)
        fh.write("\n")


def next_version(raw):
    try:
        return str(int(raw) + 1)
    except Exception:
        return "1"


def ensure_record(store, key):
    record = store.get("records", {}).get(key)
    if record is None:
        raise RuntimeError(f"record {key!r} is not present")
    record.setdefault("version", "0")
    record.setdefault("value", {})
    return record


def maybe_reclaim_item(item):
    if item.get("state") != "reserved":
        return
    if item.get("expiry_policy") != "reclaim":
        return
    expires_at = parse_time(item.get("expires_at"))
    if expires_at is None or expires_at > now_utc():
        return
    item["state"] = "available"
    item["claim_id"] = ""
    item["expires_at"] = ""
    item["expiry_policy"] = ""
    item["version"] = next_version(item.get("version", "0"))


def selector_matches(item, selector):
    if not selector:
        return True
    selector_id = selector.get("id") or ""
    if selector_id and item.get("id") != selector_id:
        return False
    fields = selector.get("fields") or {}
    item_fields = item.get("fields") or {}
    for key, want in fields.items():
        if str(item_fields.get(key)) != str(want):
            return False
    return True


def locate_claim_item(store, claim):
    pool_name = claim.get("pool") or ""
    pool = store.get("pools", {}).get(pool_name)
    if pool is None:
        raise RuntimeError(f"pool {pool_name!r} is not present")
    for item in pool.get("items", []):
        if item.get("id") != claim.get("item_id"):
            continue
        if item.get("claim_id") != claim.get("claim_id"):
            continue
        return item
    raise RuntimeError("claim is no longer active")


def state_read(params):
    path = (params.get("config") or {}).get("path") or ""
    key = params.get("key") or ""
    store = load_store(path)
    record = ensure_record(store, key)
    return {
        "snapshot": {
            "key": key,
            "value": copy.deepcopy(record.get("value") or {}),
            "version": str(record.get("version", "0")),
            "guarantee": "local-atomic",
        }
    }


def state_cas(params):
    path = (params.get("config") or {}).get("path") or ""
    key = params.get("key") or ""
    expected = params.get("expected_version") or ""
    payload = copy.deepcopy((params.get("value") or {}))
    store = load_store(path)
    record = ensure_record(store, key)
    current = str(record.get("version", "0"))
    if current != expected:
        raise RuntimeError(f"record {key!r} version mismatch: got {current} want {expected}")
    next_ver = next_version(current)
    record["version"] = next_ver
    record["value"] = payload
    save_store(path, store)
    return {
        "snapshot": {
            "key": key,
            "value": payload,
            "version": next_ver,
            "guarantee": "local-atomic",
        }
    }


def state_claim(params):
    path = (params.get("config") or {}).get("path") or ""
    pool_name = params.get("pool") or ""
    selector = params.get("selector") or {}
    lease = params.get("lease") or {}
    ttl_ns = int(lease.get("ttl", 0))
    if ttl_ns <= 0:
        raise RuntimeError("lease.ttl must be positive")
    expiry_policy = lease.get("expiry_policy") or "stale"

    store = load_store(path)
    pool = store.get("pools", {}).get(pool_name)
    if pool is None:
        raise RuntimeError(f"pool {pool_name!r} is not present")

    chosen = None
    for item in pool.get("items", []):
        maybe_reclaim_item(item)
        if item.get("state") != "available":
            continue
        if not selector_matches(item, selector):
            continue
        chosen = item
        break

    if chosen is None:
        raise RuntimeError(f"pool {pool_name!r} has no available fixtures")

    expires_at = now_utc() + dt.timedelta(microseconds=ttl_ns / 1000)
    claim_id = f"claim-{time.time_ns()}"
    version = next_version(chosen.get("version", "0"))
    chosen["state"] = "reserved"
    chosen["claim_id"] = claim_id
    chosen["expires_at"] = encode_time(expires_at)
    chosen["expiry_policy"] = expiry_policy
    chosen["version"] = version
    save_store(path, store)

    return {
        "result": {
            "item": copy.deepcopy(chosen.get("fields") or {}),
            "claim": {
                "pool": pool_name,
                "item_id": chosen.get("id"),
                "claim_id": claim_id,
                "expires_at": encode_time(expires_at),
                "version": version,
                "expiry_policy": expiry_policy,
                "guarantee": "local-atomic",
            },
        }
    }


def state_renew(params):
    path = (params.get("config") or {}).get("path") or ""
    ttl_ns = int(params.get("ttl", 0))
    if ttl_ns <= 0:
        raise RuntimeError("ttl must be positive")
    claim = params.get("claim") or {}
    store = load_store(path)
    item = locate_claim_item(store, claim)
    expires_at = now_utc() + dt.timedelta(microseconds=ttl_ns / 1000)
    version = next_version(item.get("version", "0"))
    item["expires_at"] = encode_time(expires_at)
    item["version"] = version
    save_store(path, store)

    renewed = dict(claim)
    renewed["expires_at"] = encode_time(expires_at)
    renewed["version"] = version
    renewed["guarantee"] = "local-atomic"
    return {"claim": renewed}


def state_release(params):
    path = (params.get("config") or {}).get("path") or ""
    claim = params.get("claim") or {}
    store = load_store(path)
    item = locate_claim_item(store, claim)
    item["state"] = "available"
    item["claim_id"] = ""
    item["expires_at"] = ""
    item["expiry_policy"] = ""
    item["version"] = next_version(item.get("version", "0"))
    save_store(path, store)
    return {}


def state_consume(params):
    path = (params.get("config") or {}).get("path") or ""
    claim = params.get("claim") or {}
    tombstone = copy.deepcopy(params.get("tombstone") or {})
    store = load_store(path)
    item = locate_claim_item(store, claim)
    item["state"] = "used"
    item["claim_id"] = ""
    item["expires_at"] = ""
    item["expiry_policy"] = ""
    item["version"] = next_version(item.get("version", "0"))
    item["tombstone"] = tombstone
    save_store(path, store)
    return {}


def main():
    active = []
    while True:
        request = read_frame()
        if request is None:
            return 0
        method = request.get("method")
        req_id = request.get("id")
        params = request.get("params") or {}

        if method == "theater.cancel":
            continue

        response = {"jsonrpc": "2.0", "id": req_id}

        try:
            if method == "theater.initialize":
                allowed = params.get("allowed_capabilities") or []
                active = [name for name in allowed if name in SUPPORTED]
                response["result"] = {
                    "plugin": PLUGIN,
                    "protocol": PROTOCOL,
                    "descriptor_digest": DIGEST,
                    "active_capabilities": active,
                }
            elif method == "theater.inventory.resolve":
                response["result"] = {"value": params.get("properties", {}).get("value")}
            elif method == "theater.action.invoke":
                capability = params.get("capability")
                properties = params.get("properties", {})
                if capability == "action.smoke.echo":
                    value = properties.get("value")
                    if value == "leak-env-error":
                        leaked = os.environ.get("THEATER_PLUGIN_HOST_SECRET", "")
                        response["error"] = plugin_error(f"host env leak {leaked}", partial={"echo": leaked})
                    else:
                        response["result"] = {"outputs": {"echo": value}}
                elif capability == "action.smoke.secret_fail":
                    secret = properties.get("secret")
                    notify("theater.log", {"message": f"secret log {secret}"})
                    response["error"] = plugin_error(
                        f"secret failure {secret}",
                        theater_code="secret_failed",
                        partial={"secret_echo": secret},
                    )
                elif capability == "action.smoke.sleep":
                    time.sleep(int(properties.get("ms", 0)) / 1000.0)
                    response["result"] = {"outputs": {}}
                elif capability == "action.smoke.validate_probe":
                    response["result"] = {
                        "outputs": {
                            "prepared": "action.smoke.validate_probe" in PREPARED
                        }
                    }
                else:
                    response["error"] = {"code": -32600, "message": f"unsupported action {capability}"}
            elif method == "theater.report.export":
                properties = params.get("properties", {})
                path = properties.get("path") or ""
                if path == "leak-report-secret":
                    response["error"] = plugin_error(f"report path leaked {path}")
                else:
                    with open(path, "w", encoding="utf-8") as fh:
                        json.dump(params.get("document"), fh, indent=2, sort_keys=True)
                        fh.write("\n")
                    response["result"] = {}
            elif method == "theater.state.read":
                response["result"] = state_read(params)
            elif method == "theater.state.cas":
                response["result"] = state_cas(params)
            elif method == "theater.state.claim":
                response["result"] = state_claim(params)
            elif method == "theater.state.renew":
                response["result"] = state_renew(params)
            elif method == "theater.state.release":
                response["result"] = state_release(params)
            elif method == "theater.state.consume":
                response["result"] = state_consume(params)
            elif method == "theater.transform.apply":
                properties = params.get("properties", {})
                value = params.get("value")
                if not isinstance(value, str):
                    raise RuntimeError(f"transform.smoke.wrap expects string value, got {type(value).__name__}")
                if value == "leak-transform-secret":
                    response["error"] = plugin_error(f"transform leaked {value}")
                prefix = properties.get("prefix") or ""
                suffix = properties.get("suffix") or ""
                if "error" not in response:
                    response["result"] = {"value": f"{prefix}{value}{suffix}"}
            elif method == "theater.matcher.check":
                properties = params.get("properties", {})
                actual = params.get("actual")
                if not isinstance(actual, str):
                    raise RuntimeError(f"matcher.smoke.equal expects string actual, got {type(actual).__name__}")
                expected = properties.get("expected")
                if actual != expected:
                    raise RuntimeError(f"expected {expected!r}, got {actual!r}")
                response["result"] = {}
            elif method == "theater.validate":
                capability = params.get("capability")
                properties = params.get("properties", {})
                diagnostics = []
                if capability == "inventory.smoke.echo" and properties.get("value") == "invalid":
                    diagnostics.append({"path": "/value", "message": "value must not be invalid"})
                if capability == "inventory.smoke.echo" and properties.get("value") == "leak-env":
                    leaked = os.environ.get("THEATER_PLUGIN_HOST_SECRET", "")
                    diagnostics.append({"path": "/value", "message": f"host env leak {leaked}"})
                if capability == "action.smoke.validate_probe" and properties.get("mode") == "assert-validate-shape":
                    errors = validate_probe_shape(params)
                    if errors:
                        diagnostics.append({"path": "/mode", "message": "shape wrong: " + ", ".join(errors)})
                    else:
                        diagnostics.append({"path": "/mode", "message": "shape ok: static, nested dynamic, list dynamic, missing absent"})
                if capability == "action.smoke.validate_probe" and properties.get("mode") == "leak-validate-secret":
                    diagnostics.append({"path": f"/{properties.get('secret')}", "message": f"validate secret leak {properties.get('secret')}"})
                if capability == "action.smoke.validate_probe" and properties.get("mode") == "leak-validate-error-secret":
                    raise RuntimeError(f"validate error secret leak {properties.get('secret')}")
                response["result"] = {"diagnostics": diagnostics}
            elif method == "theater.prepare":
                capability = params.get("capability")
                properties = params.get("properties", {})
                if capability == "action.smoke.validate_probe" and properties.get("mode") == "assert-prepare-shape":
                    errors = validate_probe_shape(params)
                    if errors:
                        raise RuntimeError("prepare shape wrong: " + ", ".join(errors))
                    PREPARED.add(capability)
                if capability == "action.smoke.validate_probe" and properties.get("mode") == "leak-prepare-secret":
                    raise RuntimeError(f"prepare secret leak {properties.get('secret')}")
                response["result"] = {}
            elif method == "theater.shutdown":
                response["result"] = {}
                write_frame(response)
                return 0
            else:
                response["error"] = {"code": -32600, "message": f"unsupported method {method}"}
        except Exception as exc:
            response["error"] = plugin_error(str(exc))

        if req_id is not None:
            write_frame(response)


if __name__ == "__main__":
    raise SystemExit(main())
