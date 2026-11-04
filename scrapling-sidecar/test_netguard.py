import unittest
from unittest import mock

import netguard


def fake_resolver(table):
    def resolve(host):
        if host not in table:
            raise OSError("no such host")
        return table[host]
    return resolve


class NetGuardTest(unittest.TestCase):
    def test_blocks_internal_targets(self):
        r = fake_resolver({"meta.evil.test": ["169.254.169.254"], "rebind.test": ["93.184.215.14", "10.0.0.5"]})
        for url in [
            "http://169.254.169.254/latest/meta-data/",
            "http://127.0.0.1:8090/",
            "http://localhost/",
            "http://[::1]/",
            "http://0.1.2.3/",
            "http://meta.evil.test/",
            "http://rebind.test/",  # any internal address in the answer set blocks
            "ftp://example.com/",
            "http://nonexistent.test/",
        ]:
            with self.subTest(url=url):
                self.assertFalse(netguard.is_public_url(url, resolve=r), url)

    def test_allows_public(self):
        r = fake_resolver({"www.reddit.com": ["151.101.1.140"]})
        self.assertTrue(netguard.is_public_url("https://www.reddit.com/r/saas/new.json", resolve=r))
        self.assertTrue(netguard.is_public_url("https://93.184.215.14/", resolve=r))

    def test_override_for_local_dev(self):
        with mock.patch.dict("os.environ", {"SIDECAR_ALLOW_PRIVATE_URLS": "1"}):
            self.assertTrue(netguard.is_public_url("http://127.0.0.1/"))



class GuardedFetchTest(unittest.TestCase):
    class Resp:
        def __init__(self, url):
            self.url = url

    def test_refuses_internal_before_fetching(self):
        calls = []
        with self.assertRaises(netguard.Blocked):
            netguard.guarded_fetch(lambda u: calls.append(u), "http://127.0.0.1/")
        self.assertEqual(calls, [])

    def test_refuses_redirect_to_internal(self):
        with self.assertRaises(netguard.Blocked):
            netguard.guarded_fetch(lambda u: self.Resp("http://169.254.169.254/latest/"), "https://93.184.215.14/")

    def test_returns_public_page(self):
        resp = netguard.guarded_fetch(lambda u: self.Resp("https://93.184.215.14/home"), "https://93.184.215.14/")
        self.assertEqual(resp.url, "https://93.184.215.14/home")


if __name__ == "__main__":
    unittest.main()
