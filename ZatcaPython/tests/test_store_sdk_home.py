"""
Per-store SDK homes: every store signs in its own ZatcaPython/store-data/<store>
folder (own config, PIH file and certificates) that links to the one shared
SDK (jar, schemas, rules), the same layout as the production server. Run from
the repository root:

    python -m unittest discover -s ZatcaPython/tests -v
"""
import os
import shutil
import sys
import unittest
import uuid

sys.path.insert(0, os.path.abspath("ZatcaPython"))

from utilities.einvoice_signer import ensure_store_sdk_home

SDK_HOME = os.path.abspath("ZatcaPython/utilities/fatoora-cli-simulation")


class StoreSdkHomeTest(unittest.TestCase):
    def setUp(self):
        self.stores = ["test-" + uuid.uuid4().hex for _ in range(2)]
        self.addCleanup(self.remove_homes)

    def remove_homes(self):
        for s in self.stores:
            shutil.rmtree(os.path.abspath(f"ZatcaPython/store-data/{s}"), ignore_errors=True)

    def test_no_store_uses_the_shared_sdk(self):
        self.assertEqual(ensure_store_sdk_home(""), SDK_HOME)

    def test_each_store_gets_its_own_home_linked_to_the_shared_sdk(self):
        a, b = (ensure_store_sdk_home(s) for s in self.stores)
        self.assertNotEqual(a, b)
        for home, store in ((a, self.stores[0]), (b, self.stores[1])):
            self.assertEqual(home, os.path.abspath(f"ZatcaPython/store-data/{store}"))
            for d in ("Configuration", "Data/PIH", "Data/Input", "Data/Certificates"):
                path = os.path.join(home, d)
                self.assertTrue(os.path.isdir(path) and not os.path.islink(path), f"{path}: own folder")
            for name in ("Apps", "Data/Schemas", "Data/Rules"):
                src = os.path.join(SDK_HOME, name)
                if os.path.exists(src):
                    link = os.path.join(home, name)
                    self.assertTrue(os.path.islink(link), f"{link}: links to the shared SDK")
                    self.assertEqual(os.path.realpath(link), os.path.realpath(src))

    def test_a_store_home_is_kept_and_its_state_stays_its_own(self):
        a = ensure_store_sdk_home(self.stores[0])
        b = ensure_store_sdk_home(self.stores[1])
        pih_a = os.path.join(a, "Data", "PIH", "pih.txt")
        with open(pih_a, "w") as f:
            f.write("store-a-hash")
        # a second call reuses the home and leaves its state alone
        self.assertEqual(ensure_store_sdk_home(self.stores[0]), a)
        with open(pih_a) as f:
            self.assertEqual(f.read(), "store-a-hash")
        pih_b = os.path.join(b, "Data", "PIH", "pih.txt")
        if os.path.exists(pih_b):
            with open(pih_b) as f:
                self.assertNotEqual(f.read(), "store-a-hash")
        # nothing a store writes lands in the shared SDK
        shared_pih = os.path.join(SDK_HOME, "Data", "PIH", "pih.txt")
        if os.path.exists(shared_pih):
            with open(shared_pih) as f:
                self.assertNotEqual(f.read(), "store-a-hash")


if __name__ == "__main__":
    unittest.main()
