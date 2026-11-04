"""SSRF guard for the sidecar: it browses user-supplied URLs from inside the
private network, so it refuses (before fetching, and again on the final URL
after redirects) anything that resolves to an internal address.

ponytail: check-then-fetch, so DNS rebinding between the check and the
browser's own lookup is still possible; network isolation is the hard fix.
"""
import ipaddress
import os
import socket
from typing import Callable, Iterable
from urllib.parse import urlsplit

_EXTRA_BLOCKED = [ipaddress.ip_network(n) for n in ("0.0.0.0/8", "100.64.0.0/10", "198.18.0.0/15")]
_NAT64 = ipaddress.ip_network("64:ff9b::/96")


def _system_resolve(host: str) -> Iterable[str]:
    return [info[4][0] for info in socket.getaddrinfo(host, None)]


def is_internal_ip(raw: str) -> bool:
    ip = ipaddress.ip_address(raw.split("%")[0])
    if isinstance(ip, ipaddress.IPv6Address):
        if ip.ipv4_mapped:
            ip = ip.ipv4_mapped
        elif ip in _NAT64:
            ip = ipaddress.IPv4Address(int(ip) & 0xFFFFFFFF)
    if ip.is_loopback or ip.is_private or ip.is_link_local or ip.is_unspecified or ip.is_multicast or ip.is_reserved:
        return True
    return any(ip in net for net in _EXTRA_BLOCKED if ip.version == net.version)


def is_public_url(url: str, resolve: Callable[[str], Iterable[str]] = _system_resolve) -> bool:
    if os.environ.get("SIDECAR_ALLOW_PRIVATE_URLS") == "1":  # local dev against mock servers only
        return True
    parts = urlsplit(url)
    if parts.scheme not in ("http", "https") or not parts.hostname:
        return False
    host = parts.hostname
    try:
        ipaddress.ip_address(host)
        addrs: Iterable[str] = [host]
    except ValueError:
        try:
            addrs = list(resolve(host))
        except OSError:
            return False
    addrs = list(addrs)
    return bool(addrs) and not any(is_internal_ip(a) for a in addrs)


class Blocked(Exception):
    """The URL, or where it redirected, is internal."""


def guarded_fetch(fetch: Callable[[str], object], url: str) -> object:
    """Run fetch(url) only for a public URL, and refuse the result if the
    browser ended up (server- or client-side redirect) on an internal one."""
    if not is_public_url(url):
        raise Blocked("url resolves to an internal address")
    resp = fetch(url)
    final = getattr(resp, "url", None) or url
    if not is_public_url(final):
        raise Blocked("page redirected to an internal address")
    return resp
