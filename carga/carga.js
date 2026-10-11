// Comporta load test, always against the fake provider (never against DeepSeek: it would cost money).
//
// Start the gateway without DEEPSEEK_API_KEY (it uses the fake) and run, for example:
//   docker run --rm -i -e CENARIO=sem_cache -e BASE=http://host.docker.internal:8080 grafana/k6 run - < carga/carga.js
//
// Scenarios (CENARIO variable):
//   sem_cache  (no cache) every request is unique: measures the gateway's overhead over the provider
//   com_cache  (cache) 20 questions repeated with X-Comporta-Cache: measures how many paid calls the cache avoids
//   caos       (chaos) same as sem_cache, but run the gateway with FALSO_TAXA_ERRO=0.5: retry, breaker and the
//              second destination of the "rapido" alias (comporta.json) absorb the errors
import http from "k6/http";
import { check } from "k6";

const BASE = __ENV.BASE || "http://host.docker.internal:8080";
const ADMIN = __ENV.ADMIN_TOKEN || "adm";
const CENARIO = __ENV.CENARIO || "sem_cache";

export const options = {
  scenarios: {
    [CENARIO]: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RPS || 500),
      timeUnit: "1s",
      duration: __ENV.DURACAO || "30s",
      preAllocatedVUs: 200,
      maxVUs: 2000,
    },
  },
  summaryTrendStats: ["avg", "p(50)", "p(95)", "p(99)", "max"],
};

const admin = { headers: { Authorization: `Bearer ${ADMIN}`, "Content-Type": "application/json" } };

function metricas() {
  const m = {};
  for (const l of http.get(`${BASE}/metrics`).body.split("\n")) {
    const [nome, valor] = l.split(" ");
    if (nome.startsWith("comporta_") && !nome.includes("{")) m[nome] = Number(valor);
  }
  return m;
}

// setup creates a client with no limits and LGPD on (masking is part of the measurement).
export function setup() {
  const cli = http.post(`${BASE}/admin/clientes`, JSON.stringify({ nome: `carga-${CENARIO}-${Date.now()}`, lgpd: true }), admin).json();
  const chave = http.post(`${BASE}/admin/clientes/${cli.id}/chaves`, null, admin).json().chave;
  return { chave, antes: metricas() };
}

export default function (d) {
  const pergunta = CENARIO === "com_cache" ? Math.floor(Math.random() * 20) : `${__VU}-${__ITER}`;
  const headers = { Authorization: `Bearer ${d.chave}`, "Content-Type": "application/json" };
  if (CENARIO === "com_cache") headers["X-Comporta-Cache"] = "ligado";
  const r = http.post(
    `${BASE}/v1/chat/completions`,
    JSON.stringify({ model: "rapido", messages: [{ role: "user", content: `question ${pergunta}, meu CPF é 529.982.247-25` }] }),
    { headers },
  );
  check(r, { "status 200": (x) => x.status === 200 });
}

export function teardown(d) {
  const depois = metricas();
  const delta = (n) => (depois[n] || 0) - (d.antes[n] || 0);
  const req = delta("comporta_requisicoes_total");
  const hits = delta("comporta_cache_hits_total");
  console.log(
    `requests=${req} errors=${delta("comporta_erros_total")} cache_hits=${hits} ` +
      `calls_avoided=${req ? ((100 * hits) / req).toFixed(1) : 0}% pii=${delta("comporta_pii_mascarados_total")} ` +
      `usage_dropped=${delta("comporta_uso_descartados_total")}`,
  );
}
