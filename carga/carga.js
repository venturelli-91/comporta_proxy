// Carga da Comporta, sempre contra o provedor falso (nunca contra a DeepSeek: custaria dinheiro).
//
// Suba o gateway sem DEEPSEEK_API_KEY (usa o falso) e rode, por exemplo:
//   docker run --rm -i -e CENARIO=sem_cache -e BASE=http://host.docker.internal:8080 grafana/k6 run - < carga/carga.js
//
// Cenários (variável CENARIO):
//   sem_cache  cada requisição é única: mede o overhead do gateway sobre o provedor
//   com_cache  20 perguntas repetidas com X-Comporta-Cache: mede quanto o cache evita de chamadas pagas
//   caos       igual ao sem_cache, mas rode o gateway com FALSO_TAXA_ERRO=0.5: retry, breaker e o
//              segundo destino do alias "rapido" (comporta.json) seguram o erro
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

// setup cria um cliente sem limite e com LGPD ligada (o mascaramento entra na medição).
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
    JSON.stringify({ model: "rapido", messages: [{ role: "user", content: `pergunta ${pergunta}, meu CPF é 529.982.247-25` }] }),
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
    `requisições=${req} erros=${delta("comporta_erros_total")} cache_hits=${hits} ` +
      `chamadas_evitadas=${req ? ((100 * hits) / req).toFixed(1) : 0}% pii=${delta("comporta_pii_mascarados_total")} ` +
      `uso_descartado=${delta("comporta_uso_descartados_total")}`,
  );
}
