#!/usr/bin/env node
// Runs the prebuilt repoanalyzer binary for this machine. The package carries
// one per platform, in bin/<platform>-<arch>/.
"use strict";

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const target = `${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "repoanalyzer.exe" : "repoanalyzer";
const bin = path.join(__dirname, target, exe);

if (!fs.existsSync(bin)) {
	console.error(
		`repoanalyzer: there's no prebuilt binary for ${target}.\n` +
			"With Go 1.24+ you can build it instead:\n" +
			"  go install github.com/mynameisgump/repoanalyzer/cmd/repoanalyzer@latest",
	);
	process.exit(1);
}

// The dashboard needs the real terminal, so stdin/stdout/stderr pass straight through.
const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (r.error) {
	console.error(`repoanalyzer: ${r.error.message}`);
	process.exit(1);
}
if (r.signal) {
	process.kill(process.pid, r.signal);
}
process.exit(r.status ?? 1);
