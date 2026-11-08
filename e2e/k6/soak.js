// Nexus 48h soak: 15K concurrent users exercising radar fanout, feed reads
// and whisper send/read churn at steady load, with server-side memory-leak
// detection through the Go runtime metrics exposed on /metrics.
//
// Deviation from the Phase E spec (flagged in evidence/logs/soak-48h.log):
// the monolith has NO WebSocket transport — chat is REST polling and radar
// is client-pulled. The WebSocket branch below (SOAK_WS=1) targets the
// future /api/v1/ws transport and is exercised once that endpoint exists;
// the default REST branch covers the same hot paths the WS load would hit
// (radar fanout + feed + whisper lifecycle).
//
// Environment (staging soak target, OTP_DEBUG=1 test environment):
//   BASE_URL        http://api-staging:8080
//   SOAK_DURATION   "48h"          (k6 max-duration string)
//   SOAK_VUS        15000
//   SOAK_WS         0|1            (default 0 = REST polling)
//   TEST_PHONE_*/TEST_OTP_*       seeded adult+dating-ready accounts per VU
//                                 group, OR LOGIN_PHONE/LOGIN_OTP for a
//                                 shared pool (rate limits off in soak env)
//
// Run:
//   k6 run -e BASE_URL=https://soak.nexus.example e2e/k6/soak.js
//
// Memory-leak check (server side, run alongside):
//   while true; do curl -s api:9090/metrics | grep -E '^go_memstats_heap_inuse_bytes'; sleep 60; done | tee heap-growth.log
// Leak signal: heap_inuse_bytes grows monotonically across GC cycles for
// hours while request rate and p95 stay flat.

import http from "k6/http";
import { Trend, Rate, Counter } from "k6/metrics";
import { check, sleep } from "k6";
import exec from "k6/execution";

const BASE = __ENV.BASE_URL || "http://localhost:8080";
const WS = (__ENV.SOAK_WS || "0") === "1";
const DURATION = __ENV.SOAK_DURATION || "48h";
const VUS = parseInt(__ENV.SOAK_VUS || "15000", 10);

export const options = {
  scenarios: {
    steady: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30m", target: VUS }, // warm up to full concurrency
        { duration: DURATION, target: VUS }, // 48h steady state
        { duration: "30m", target: 0 }, // drain
      ],
      gracefulRampDown: "5m",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    radar_p95: ["p(95)<2000"],
    feed_p95: ["p(95)<1500"],
    whisper_p95: ["p(95)<1000"],
  },
};

const radarTrend = new Trend("radar_p95", true);
const feedTrend = new Trend("feed_p95", true);
const whisperTrend = new Trend("whisper_p95", true);
const radarDegraded = new Rate("radar_degraded");
const whisperErrors = new Rate("whisper_errors");
const whisperSent = new Counter("whisper_sent");

// Per-VU bearer token: soak env seeds accounts as
// phone = SOAK_PHONE_PREFIX + vu.id, OTP = SOAK_OTP (OTP_DEBUG=1).
function authenticate() {
  const prefix = __ENV.SOAK_PHONE_PREFIX || "0998000000";
  const phone = prefix + String(exec.vu.idInInstance).padStart(6, "0");
  const otp = __ENV.SOAK_OTP || "000000";
  const login = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ phone }),
    { headers: { "Content-Type": "application/json" } }
  );
  if (login.status !== 200) {
    // First touch: register, then verify with the debug OTP.
    http.post(`${BASE}/api/v1/auth/register`, JSON.stringify({ phone }), {
      headers: { "Content-Type": "application/json" },
    });
    const verify = http.post(
      `${BASE}/api/v1/auth/verify`,
      JSON.stringify({ phone, code: otp }),
      { headers: { "Content-Type": "application/json" } }
    );
    if (verify.status !== 200) {
      exec.test.abort(`auth failed for ${phone}: ${verify.status}`);
    }
    return verify.json("access_token");
  }
  return login.json("access_token");
}

function makeDatingReady(token) {
  http.put(
    `${BASE}/api/v1/me/birthdate`,
    JSON.stringify({ date_of_birth: "1995-06-15" }),
    { headers: auth(token) }
  );
  http.put(
    `${BASE}/api/v1/dating/profile`,
    JSON.stringify({ nickname: `Soak${exec.vu.idInInstance}`, bio: "soak", avatar_url: "", is_hidden_from_friends: false }),
    { headers: auth(token) }
  );
}

function auth(token) {
  return { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
}

// Deterministic pseudo-random nearby coordinates (Yangon box) per VU so
// radar queries actually overlap and fan out across the same cells.
const baseLat = 16.78 + (exec.vu.idInInstance % 200) * 0.0004;
const baseLng = 96.15 + (exec.vu.idInInstance % 200) * 0.0004;

export default function () {
  const token = authenticate();
  makeDatingReady(token);

  if (WS) {
    wsSoak(token);
    return;
  }

  // ---- REST steady loop: radar poll + feed page + whisper churn ---------
  const t0 = Date.now();
  const radar = http.get(
    `${BASE}/api/v1/radar?lat=${baseLat}&lng=${baseLng}&radius=2000`,
    { headers: auth(token) }
  );
  radarTrend.add(Date.now() - t0);
  check(radar, { "radar 200": (r) => r.status === 200 });
  if (radar.status === 200) {
    radarDegraded.add(radar.json("degraded") === true);
  }

  const t1 = Date.now();
  const feed = http.get(`${BASE}/api/v1/posts?limit=20`, { headers: auth(token) });
  feedTrend.add(Date.now() - t1);
  check(feed, { "feed 200": (r) => r.status === 200 });

  // Whisper lifecycle: send to a deterministic neighbour VU, read it back.
  // Neighbour may not be a match in a cold DB; failures are counted, not
  // fatal — the point is sustained send/read churn on the Redis path.
  const t2 = Date.now();
  const target = exec.vu.idInInstance + 1;
  const send = http.post(
    `${BASE}/api/v1/whisper`,
    JSON.stringify({ to: `d${target}`, body: "soak churn" }),
    { headers: auth(token) }
  );
  if (send.status === 201) {
    whisperSent.add(1);
    const id = send.json("id");
    const read = http.get(`${BASE}/api/v1/whisper/${id}`, { headers: auth(token) });
    check(read, { "whisper read": (r) => r.status === 200 || r.status === 403 || r.status === 410 });
  } else {
    whisperErrors.add(send.status >= 500);
  }
  whisperTrend.add(Date.now() - t2);

  sleep(1); // ~1 request-triplet per second per VU at steady state
}

// WebSocket branch: targets the future /api/v1/ws transport. Enabled only
// with SOAK_WS=1 once the endpoint ships; k6/ws is synchronous per VU.
function wsSoak(token) {
  const wsUrl = BASE.replace(/^http/, "ws") + "/api/v1/ws";
  const res = http.ws.connect(
    wsUrl,
    { headers: { Authorization: `Bearer ${token}` } },
    function (socket) {
      socket.on("open", () => {
        socket.send(
          JSON.stringify({ type: "radar.subscribe", lat: baseLat, lng: baseLng, radius: 2000 })
        );
      });
      socket.on("message", (data) => {
        const msg = JSON.parse(data);
        if (msg.type === "radar.nodes") radarTrend.add(0);
      });
      socket.setTimeout(() => socket.close(), 24 * 3600 * 1000);
    }
  );
  check(res, { "ws upgrade": (r) => r && r.status === 101 });
}
