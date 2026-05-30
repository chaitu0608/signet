#!/usr/bin/env node
/**
 * EIP-712 sign a git push for ForgePulse.
 * Usage: node sign-push.mjs <repo> <ref> <old_sha> <new_sha>
 * Env: FORGEPULSE_PRIVATE_KEY or ~/.forgepulse/key
 * Output: JSON { "signer", "signature" }
 */
import { readFileSync, existsSync } from "fs";
import { homedir } from "os";
import { join } from "path";
import { ethers } from "ethers";

const [,, repo, ref, oldSha, newSha] = process.argv;
if (!repo || !ref || !newSha) {
  console.log("{}");
  process.exit(0);
}

function loadKey() {
  if (process.env.FORGEPULSE_PRIVATE_KEY) {
    return process.env.FORGEPULSE_PRIVATE_KEY.trim();
  }
  const p = join(homedir(), ".forgepulse", "key");
  if (existsSync(p)) {
    return readFileSync(p, "utf8").trim();
  }
  return null;
}

const pk = loadKey();
if (!pk) {
  console.log("{}");
  process.exit(0);
}

const chainId = parseInt(process.env.CHAIN_ID || "84532", 10);
const anchor = process.env.ANCHOR_ADDR || "0x0000000000000000000000000000000000000000";

const wallet = new ethers.Wallet(pk.startsWith("0x") ? pk : `0x${pk}`);
const now = Math.floor(Date.now() / 1000);

const domain = {
  name: "ForgePulse",
  version: "1",
  chainId,
  verifyingContract: anchor,
};

const types = {
  PushEvent: [
    { name: "repo", type: "string" },
    { name: "ref", type: "string" },
    { name: "oldSha", type: "bytes32" },
    { name: "newSha", type: "bytes32" },
    { name: "nonce", type: "uint64" },
    { name: "chainTimestamp", type: "uint64" },
  ],
};

function padSha(s) {
  if (!s || s.startsWith("0000000")) {
    return ethers.ZeroHash;
  }
  const hex = s.length >= 64 ? s.slice(0, 64) : s.padStart(64, "0");
  return "0x" + hex.replace(/^0x/, "").slice(0, 64).padStart(64, "0");
}

const value = {
  repo,
  ref,
  oldSha: padSha(oldSha || ""),
  newSha: padSha(newSha),
  nonce: BigInt(now) * 1000000n,
  chainTimestamp: BigInt(now),
};

try {
  const signature = await wallet.signTypedData(domain, types, value);
  console.log(JSON.stringify({ signer: wallet.address, signature }));
} catch (e) {
  console.log("{}");
}
