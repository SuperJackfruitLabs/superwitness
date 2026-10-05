// A stand-in AgentPod hub for the end-to-end test, on 127.0.0.1:8791. It publishes a JWKS,
// answers the authorize redirect at once for whoever /__stub/sign-in-as last named, and
// exchanges each code once for a signed token, checking PKCE as the hub does. Node only.
import { createHash, generateKeyPairSync, randomBytes, sign } from "node:crypto";
import { createServer } from "node:http";

const PORT = 8791;
const ISSUER = `http://127.0.0.1:${PORT}`;
const AUDIENCE = "http://127.0.0.1:8790";
const { publicKey, privateKey } = generateKeyPairSync("ed25519");
const jwk = publicKey.export({ format: "jwk" });
const codes = new Map();
let who = { sub: "hubuser_01", kind: "human", email: "human01@example.com" };

const b64 = (b) => Buffer.from(b).toString("base64url");
const sha256 = (s) => b64(createHash("sha256").update(s).digest());

function mint(c) {
  const now = Math.floor(Date.now() / 1000);
  const header = b64(JSON.stringify({ alg: "EdDSA", kid: "stub1", typ: "JWT" }));
  const claims = { iss: ISSUER, aud: [AUDIENCE], sub: c.sub, principalKind: c.kind, tenant: "tenant_01", iat: now, exp: now + 300 };
  if (c.email) Object.assign(claims, { email: c.email, email_verified: true });
  const payload = b64(JSON.stringify(claims));
  return `${header}.${payload}.${b64(sign(null, Buffer.from(`${header}.${payload}`), privateKey))}`;
}

function json(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(body === undefined ? "" : JSON.stringify(body));
}

async function readBody(req) {
  let s = "";
  for await (const chunk of req) s += chunk;
  return s;
}

createServer(async (req, res) => {
  const url = new URL(req.url, ISSUER);
  if (req.method === "GET" && url.pathname === "/api/auth/jwks") {
    return json(res, 200, { keys: [{ ...jwk, alg: "EdDSA", kid: "stub1" }] });
  }
  if (req.method === "POST" && url.pathname === "/__stub/sign-in-as") {
    who = JSON.parse(await readBody(req));
    return json(res, 200, who);
  }
  if (req.method === "GET" && url.pathname === "/api/auth/authorize") {
    const q = url.searchParams;
    if (!q.get("client") || q.get("code_challenge_method") !== "S256" || (q.get("code_challenge") ?? "").length !== 43 || !q.get("state")) {
      return json(res, 400, { error: "refused" });
    }
    const code = b64(randomBytes(16));
    codes.set(code, { challenge: q.get("code_challenge"), redirect: q.get("redirect_uri"), ...who });
    const back = new URL(q.get("redirect_uri"));
    back.searchParams.set("code", code);
    back.searchParams.set("state", q.get("state"));
    res.writeHead(302, { Location: back.toString() });
    return res.end();
  }
  if (req.method === "POST" && url.pathname === "/api/auth/token/exchange") {
    if (req.headers.origin) return json(res, 400, { error: "invalid_request" });
    const b = JSON.parse(await readBody(req));
    const c = codes.get(b.code);
    codes.delete(b.code);
    if (!c || c.redirect !== b.redirect_uri || sha256(b.code_verifier ?? "") !== c.challenge) return json(res, 400, { error: "invalid_grant" });
    return json(res, 200, { token: mint(c), expiresIn: 300 });
  }
  json(res, 404, { error: "not_found" });
}).listen(PORT, "127.0.0.1");
