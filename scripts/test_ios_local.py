"""Offline Makefile contract tests. All Go/mobile commands are fake; no bind coverage."""

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

CORE = Path(__file__).resolve().parents[1]
MAKE = shutil.which("make")
KEYS = "HOME GOPATH GOBIN GOMODCACHE GOCACHE GOTMPDIR TMPDIR CLANG_MODULE_CACHE_PATH XDG_CACHE_HOME GOTOOLCHAIN GOFLAGS GOENV GOWORK GOTELEMETRY GOPROXY GOSUMDB GOVCS GOAUTH IPHONEOS_DEPLOYMENT_TARGET".split()
FAKE = r'''
import json, os, pathlib, sys
name, args = pathlib.Path(sys.argv[0]).name, sys.argv[1:]
with open(os.environ["FAKE_LOG"], "a") as log:
    log.write(json.dumps({"name": name, "args": args, "env": dict(os.environ)}) + "\n")
if name == "go" and args == ["env", "GOVERSION"]:
    print(os.environ.get("FAKE_GO_VERSION", "go1.26.3"))
elif name == "go" and len(args) == 3 and args[:2] == ["version", "-m"]:
    tool = pathlib.Path(args[2]).name
    version = "v0.1.11" if os.environ.get("FAKE_BAD_TOOL") == tool else "v0.1.12"
    print(f"{args[2]}: go1.26.3\n\tpath\tgithub.com/sagernet/gomobile/cmd/{tool}\n\tmod\tgithub.com/sagernet/gomobile\t{version}\th1:fake")
elif name == "gomobile" and args[:1] == ["bind"]:
    out = pathlib.Path(args[args.index("-o") + 1])
    out.mkdir()
    (out / "Info.plist").write_text("fake generated manifest")
    sys.exit(int(os.environ.get("FAKE_BIND_EXIT", "0")))
else:
    sys.exit("Unexpected command; real tools are forbidden: " + repr(sys.argv))
'''


class IosLocalTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix="ios-local-fake-")
        self.addCleanup(temp.cleanup)
        self.project = Path(temp.name).resolve()
        self.root = self.project / "private build"
        self.go = self.root / "sdk/bin/go"
        self.log = self.project / "fake.jsonl"
        for tool in [self.go, *(self.root / "tools/bin" / name for name in ("gomobile", "gobind"))]:
            tool.parent.mkdir(parents=True, exist_ok=True)
            tool.write_text(f"#!{sys.executable}\n" + FAKE)
            tool.chmod(0o700)
        (self.project / "go.mod").write_text((CORE / "go.mod").read_text())
        (self.project / "Info.plist").write_text("legacy manifest must not be copied")
        # Do not inherit secrets, Make overrides, or real tool paths. Even Makefile
        # parsing must not run the optional legacy go run encryption command.
        self.env = {"PATH": f"{self.go.parent}:/usr/bin:/bin", "FAKE_LOG": str(self.log)}

    def run_make(self, expected=0, extra=(), **env):
        result = subprocess.run(
            [MAKE, "--no-print-directory", "-C", str(self.project), "-f", str(CORE / "Makefile"),
             "ios-local", f"IOS_LOCAL_ROOT={self.root}", f"IOS_LOCAL_GO={self.go}",
             "CRONET_GO_VERSION=fake", "HIDDIFY_SECRET_KEY=", "PSIPHON_CONFIG=", *extra],
            env=self.env | env, text=True, capture_output=True, timeout=15,
        )
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        return result.stdout + result.stderr

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_bind_contract_and_private_environment(self):
        self.run_make(GOFLAGS="-mod=mod", GOTOOLCHAIN="auto")
        calls = self.calls()
        self.assertEqual([call["name"] for call in calls], ["go", "go", "go", "gomobile"])
        args = calls[-1]["args"]
        makefile = (CORE / "Makefile").read_text()
        tags = [re.search(rf"^{name}=(.+)$", makefile, re.M)[1] for name in ("TAGS", "IOS_ADD_TAGS")]
        self.assertIn("-tags=" + ",".join(tags), args)
        self.assertEqual(args[args.index("-target") + 1], "ios")
        self.assertIn("-iosversion=15.5", args)
        self.assertIn("-libname=hiddify-core", args)
        self.assertIn("-trimpath", args)
        self.assertTrue(any(arg.startswith("-ldflags=-w -s -checklinkname=0 -buildid=") for arg in args))
        self.assertEqual(args[-2:], ["github.com/sagernet/sing-box/experimental/libbox", "./platform/mobile"])
        for call in calls:
            env = call["env"]
            for key in KEYS[:9]:
                self.assertTrue(env[key].startswith(str(self.root) + "/"), key)
            self.assertEqual(env["GOTOOLCHAIN"], "local")
            self.assertEqual(env["GOFLAGS"], "-mod=readonly")
            for key in ("GOENV", "GOWORK", "GOTELEMETRY", "GOPROXY", "GOSUMDB", "GOAUTH"):
                self.assertEqual(env[key], "off", key)
            self.assertEqual(env["GOVCS"], "*:off")
            self.assertEqual(env["IPHONEOS_DEPLOYMENT_TARGET"], "15.5")
            self.assertEqual(env["HIDDIFY_SECRET_KEY"], "")
            self.assertEqual(env["PSIPHON_CONFIG"], "")
        output = self.root / "out/HiddifyCore.xcframework"
        self.assertEqual(args[args.index("-o") + 1], str(output))
        self.assertEqual((output / "Info.plist").read_text(), "fake generated manifest")

    def test_existing_directory_is_preserved(self):
        out = self.root / "out"
        out.mkdir()
        (out / "keep").write_bytes(b"unchanged")
        self.assertIn("refusing to overwrite", self.run_make(expected=2))
        self.assertEqual((out / "keep").read_bytes(), b"unchanged")
        self.assertEqual(self.calls(), [])

    def test_existing_file_is_preserved(self):
        out = self.root / "out"
        out.write_bytes(b"unchanged")
        self.run_make(expected=2)
        self.assertEqual(out.read_bytes(), b"unchanged")
        self.assertEqual(self.calls(), [])

    def test_dangling_output_link_is_not_followed(self):
        out = self.root / "out"
        out.symlink_to("absent")
        self.run_make(expected=2)
        self.assertEqual(os.readlink(out), "absent")
        self.assertEqual(self.calls(), [])

    def test_wrong_go_version_stops_before_bind(self):
        self.assertIn("exactly go1.26.3", self.run_make(expected=2, FAKE_GO_VERSION="go1.26.2"))
        self.assertEqual(len(self.calls()), 1)

    def test_wrong_gobind_version_stops_before_bind(self):
        self.assertIn("gobind must be gomobile v0.1.12", self.run_make(expected=2, FAKE_BAD_TOOL="gobind"))
        self.assertEqual(len(self.calls()), 3)

    def test_missing_tool_stops_before_go(self):
        (self.root / "tools/bin/gobind").unlink()
        self.assertIn("missing private executable", self.run_make(expected=2))
        self.assertEqual(self.calls(), [])

    def test_missing_root_is_not_created(self):
        missing = self.project / "missing"
        self.run_make(expected=2, extra=[f"IOS_LOCAL_ROOT={missing}"])
        self.assertFalse(missing.exists())
        self.assertEqual(self.calls(), [])

    def test_mixed_goals_are_rejected_before_tools(self):
        self.assertIn("must be invoked by itself", self.run_make(expected=2, extra=["ios"]))
        self.assertEqual(self.calls(), [])

    def test_failed_bind_output_is_retained_and_not_retried(self):
        self.run_make(expected=2, FAKE_BIND_EXIT="3")
        before = self.calls()
        self.run_make(expected=2)
        self.assertEqual(self.calls(), before)
        self.assertEqual((self.root / "out/HiddifyCore.xcframework/Info.plist").read_text(), "fake generated manifest")


if __name__ == "__main__":
    unittest.main(verbosity=2)
