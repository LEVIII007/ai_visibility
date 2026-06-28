#!/usr/bin/env python3
"""
attribution_spike.py

Validates the make-or-break assumption (R1 in TRD.md):
  Can plain process + socket enumeration on macOS attribute outbound
  connections to AI tools WITHOUT an Apple NetworkExtension entitlement?

Method: shell out to `lsof` (no sudo needed for the CURRENT USER's processes,
which is exactly where the AI desktop apps live).

Two orthogonal axes, exactly as TRD.md describes:
  WHO   -> process command name        (the strong, on-device-only signal)
  WHERE -> remote host via reverse DNS  (the weak / best-effort signal)

This is a throwaway SPIKE. Its job is to MEASURE how clean attribution is and
report the unattributable residue honestly -- not to be the agent.
"""

import concurrent.futures
import ipaddress
import socket
import subprocess
import sys
from collections import defaultdict

# WHO axis: substring (lowercased) in process command -> tool identity
PROCESS_CATALOG = [
    ("cursor",  "Cursor"),
    ("claude",  "Claude"),
    ("chatgpt", "ChatGPT"),
    ("codex",   "OpenAI Codex"),
    ("copilot", "GitHub Copilot"),
    ("perplexity", "Perplexity"),
    ("raycast", "Raycast AI"),
    ("codeium", "Codeium/Windsurf"),
    ("windsurf","Codeium/Windsurf"),
    ("ollama",  "Ollama (local)"),
    ("warp",    "Warp"),
]

# WHERE axis: substring (lowercased) in reverse-DNS hostname -> provider
DEST_CATALOG = [
    ("anthropic",          "Anthropic"),
    ("openai",             "OpenAI"),
    ("oaistatic",          "OpenAI"),
    ("githubcopilot",      "GitHub Copilot"),
    ("generativelanguage", "Google Gemini"),
    ("cursor",             "Cursor"),
    ("codeium",            "Codeium"),
    ("perplexity",         "Perplexity"),
]

# WHERE axis, no-entitlement tier: published provider IP ranges.
# ILLUSTRATIVE ONLY — a real catalog pulls each provider's published CIDR blocks.
# (Anthropic announces 160.79.104.0/23. Most others front via Azure/Cloudflare/GCP
#  and need their published ranges, which is exactly the catalog maintenance work.)
IP_CATALOG = [
    ("160.79.104.0/23", "Anthropic"),
]
_IP_NETS = [(ipaddress.ip_network(c), lbl) for c, lbl in IP_CATALOG]


def match_ip(host):
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        return None
    for net, label in _IP_NETS:
        if ip.version == net.version and ip in net:
            return label
    return None


# processes we don't want cluttering the "mushiness" residue (OS chatter to 443)
SYSTEM_NOISE = ("rapportd", "apsd", "trustd", "cloudd", "nsurlsessiond",
                "identityservicesd", "secd", "akd", "mdnsresponder", "timed",
                "networkserviceproxy", "com.apple.", "icloud", "appstore",
                "softwareupd", "syncdefaul")


def rev(ip):
    try:
        return socket.gethostbyaddr(ip)[0]
    except Exception:
        return None


def run_lsof(args):
    try:
        out = subprocess.run(["/usr/sbin/lsof"] + args,
                             capture_output=True, text=True, timeout=30)
        return out.stdout
    except Exception as e:
        print(f"lsof failed ({args}): {e}", file=sys.stderr)
        return ""


def parse_field_output(text):
    """Parse `lsof -F pcn` field output -> list of (command, pid, remote_ep)."""
    conns, cmd, pid = [], None, None
    for line in text.splitlines():
        if not line:
            continue
        tag, val = line[0], line[1:]
        if tag == "p":
            pid = val
        elif tag == "c":
            cmd = val
        elif tag == "n" and "->" in val:
            remote = val.split("->", 1)[1].strip()
            conns.append((cmd, pid, remote))
    return conns


def split_host_port(ep):
    if ep.startswith("["):              # IPv6: [::1]:443
        host, _, port = ep[1:].partition("]:")
        return host, port
    host, _, port = ep.rpartition(":")  # IPv4: 1.2.3.4:443
    return host, port


def match(catalog, text):
    if not text:
        return None
    low = text.lower()
    for needle, label in catalog:
        if needle in low:
            return label
    return None


def main():
    socket.setdefaulttimeout(1.0)

    tcp = parse_field_output(run_lsof(["+c", "0", "-nP", "-iTCP",
                                       "-sTCP:ESTABLISHED", "-F", "pcn"]))
    udp = parse_field_output(run_lsof(["+c", "0", "-nP", "-iUDP", "-F", "pcn"]))

    parsed, ips = [], set()
    for cmd, pid, remote in tcp:
        host, port = split_host_port(remote)
        parsed.append((cmd, pid, host, port))
        ips.add(host)

    # best-effort reverse DNS (cap the work so the spike stays fast)
    to_resolve = list(ips)[:256]
    ptr = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=32) as ex:
        futs = {ex.submit(rev, ip): ip for ip in to_resolve}
        for f in concurrent.futures.as_completed(futs):
            ptr[futs[f]] = f.result()
    resolved = sum(1 for v in ptr.values() if v)

    by_process = defaultdict(lambda: defaultdict(int))    # tool -> host -> count
    by_dest_only = defaultdict(lambda: defaultdict(int))  # provider -> (cmd,host) -> n
    residue = defaultdict(int)                            # cmd -> count
    residue_hosts = defaultdict(set)

    for cmd, pid, host, port in parsed:
        who = match(PROCESS_CATALOG, cmd)
        hostname = ptr.get(host)
        where = match(DEST_CATALOG, hostname) or match_ip(host)
        if who:
            by_process[who][host] += 1
        elif where:
            by_dest_only[where][(cmd, hostname or host)] += 1
        elif port == "443" and not any(n in (cmd or "").lower() for n in SYSTEM_NOISE):
            residue[cmd] += 1
            if hostname:
                residue_hosts[cmd].add(hostname)

    quic = sum(1 for _, _, remote in udp if remote.endswith(":443"))
    nproc = len({p for _, p, _, _ in parsed})

    print("=" * 70)
    print("  AI-TOOL ATTRIBUTION SPIKE   (macOS · no entitlement · no sudo)")
    print("=" * 70)
    print(f"\nScanned {len(parsed)} established TCP connections "
          f"across {nproc} processes.\n")

    print("[A] Attributed to an AI tool by PROCESS NAME  (the strong signal)")
    if by_process:
        for tool, hosts in sorted(by_process.items(),
                                  key=lambda kv: -sum(kv[1].values())):
            total = sum(hosts.values())
            sample = ", ".join((ptr.get(h) or h) for h in list(hosts)[:2])
            print(f"    {tool:<18} {total:>3} conns   e.g. {sample}")
    else:
        print("    (none — open Cursor / Claude / ChatGPT desktop and re-run)")

    print("\n[B] Attributed by DESTINATION only  (process ambiguous, host told us)")
    if by_dest_only:
        for prov, items in by_dest_only.items():
            for (cmd, host), n in items.items():
                print(f"    {prov:<14} via {cmd:<20} -> {host}  ({n})")
    else:
        print("    (none resolved — see reverse-DNS hit rate below)")

    print("\n[C] Unattributed HTTPS(443) from non-system processes  (THE MUSHINESS)")
    if residue:
        for cmd, n in sorted(residue.items(), key=lambda kv: -kv[1])[:12]:
            hs = ", ".join(list(residue_hosts[cmd])[:3]) or "no usable PTR"
            print(f"    {(cmd or '?')[:30]:<30} {n:>3} conns   [{hs}]")
    else:
        print("    (none)")

    print("\n[D] Blind spots")
    print(f"    QUIC (UDP/443, rough count): ~{quic} sockets "
          f"— invisible to proxy-less capture")

    pct = (100 * resolved // len(to_resolve)) if to_resolve else 0
    print("\n" + "-" * 70)
    print(f"Reverse-DNS hit rate: {resolved}/{len(to_resolve)} unique remote IPs "
          f"resolved ({pct}%).")
    print("Low hit rate => destination-by-IP is mushy => PROCESS axis is load-bearing.")
    print("-" * 70)


if __name__ == "__main__":
    main()
