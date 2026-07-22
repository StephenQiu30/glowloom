from __future__ import annotations

import sys
import unittest
from pathlib import Path


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "packages/core/src"))
sys.path.insert(0, str(BACKEND / "packages/adapters/src"))


class PasswordHasherTests(unittest.TestCase):
    def test_argon2id_hash_round_trip_does_not_store_plaintext(self) -> None:
        from thief_adapters.identity.passwords import Argon2PasswordHasher
        from thief_core.identity import PasswordHasher

        hasher = Argon2PasswordHasher()
        self.assertIsInstance(hasher, PasswordHasher)

        encoded = hasher.hash("safe password")

        self.assertTrue(encoded.startswith("$argon2id$"))
        self.assertNotIn("safe password", encoded)
        self.assertTrue(hasher.verify(encoded, "safe password"))
        self.assertFalse(hasher.verify(encoded, "wrong password"))


if __name__ == "__main__":
    unittest.main()
