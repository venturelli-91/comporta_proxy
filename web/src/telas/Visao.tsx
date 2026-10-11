import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, num, usd, type EstadoDestino, type Relatorio } from "../api";
import { botao, botaoSecundario, cartao, Selo } from "../ui";
import { Estado } from "./Estado";

export default function Visao() {
  const uso = useQuery({ queryKey: ["uso"], queryFn: () => api<Relatorio>("/admin/uso"), refetchInterval: 5000 });
  const prov = useQuery({
    queryKey: ["provedores"],
    queryFn: () => api<EstadoDestino[]>("/admin/provedores"),
    refetchInterval: 5000,
  });
  if (!uso.data) return <Estado q={uso} />;
  const t = uso.data.total;
  if (t.requisicoes === 0)
    return (
      <div className={`${cartao} max-w-xl space-y-4 p-6`}>
        <h2 className="font-semibold">No calls in the last 24 hours</h2>
        <p className="text-sm text-suave">
          Create a client to get a <code className="font-mono">ck_</code> key and send the first call from the Playground.
          The numbers, the chart and the provider state show up here right after.
        </p>
        <div className="flex flex-wrap gap-3">
          <a className={botao} href="#clientes">
            Create a client
          </a>
          <a className={botaoSecundario} href="#playground">
            Open the Playground
          </a>
        </div>
      </div>
    );

  const taxaErro = (100 * t.erros) / t.requisicoes;
  return (
    <div className="space-y-10">
      <Vazao r={uso.data} />

      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <Numero rotulo="Spend" valor={usd(t.custo_micro)} />
        <Numero rotulo="Cache savings" valor={usd(t.economia_micro)} />
        <Numero rotulo="p95 latency" valor={`${num(t.p95_ms)} ms`} />
        <Numero rotulo="Error rate" valor={`${taxaErro.toFixed(1)}%`} tom={taxaErro >= 5 ? "erro" : t.erros > 0 ? "alerta" : undefined} />
        <Numero rotulo="Personal data protected" valor={num(t.pii)} tom={t.pii > 0 ? "acento" : undefined} largo />
      </dl>

      <section aria-labelledby="g-minuto">
        <h2 id="g-minuto" className="mb-3 font-medium">
          Requests per minute
        </h2>
        <div className={`${cartao} h-60 p-3 pt-5`}>
          <ResponsiveContainer>
            <AreaChart
              data={uso.data.serie.map((p) => ({
                ...p,
                hora: new Date(p.minuto * 1000).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
              }))}
            >
              <defs>
                <linearGradient id="preenchimento" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--acento)" stopOpacity={0.28} />
                  <stop offset="100%" stopColor="var(--acento)" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="var(--borda)" vertical={false} />
              <XAxis dataKey="hora" stroke="var(--suave)" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis stroke="var(--suave)" fontSize={12} allowDecimals={false} width={36} tickLine={false} axisLine={false} />
              <Tooltip
                cursor={{ stroke: "var(--suave)", strokeDasharray: "3 3" }}
                contentStyle={{
                  background: "var(--superficie)",
                  border: "1px solid var(--borda)",
                  borderRadius: 8,
                  boxShadow: "var(--sombra)",
                  fontSize: 13,
                }}
                labelStyle={{ color: "var(--texto)", fontWeight: 500 }}
                itemStyle={{ color: "var(--texto)" }}
              />
              <Area dataKey="requisicoes" name="Requests" stroke="var(--acento)" strokeWidth={2} fill="url(#preenchimento)" />
              <Area dataKey="erros" name="Errors" stroke="var(--erro)" strokeWidth={1.5} fill="transparent" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </section>

      <div className="grid gap-10 lg:grid-cols-[2fr_1fr]">
        <section aria-labelledby="t-clientes" className="min-w-0">
          <h2 id="t-clientes" className="mb-3 font-medium">
            By client
          </h2>
          <div className={`${cartao} overflow-x-auto`}>
            <table className="w-full text-sm">
              <thead className="text-left text-suave">
                <tr className="border-b border-borda">
                  <th className="px-4 py-2.5 font-medium">Client</th>
                  <th className="px-4 py-2.5 text-right font-medium">Requests</th>
                  <th className="px-4 py-2.5 text-right font-medium">Cache</th>
                  <th className="px-4 py-2.5 text-right font-medium">Spend</th>
                  <th className="px-4 py-2.5 text-right font-medium">Errors</th>
                </tr>
              </thead>
              <tbody>
                {uso.data.por_cliente.map((g) => (
                  <tr key={g.nome} className="border-b border-borda last:border-0 hover:bg-realce">
                    <td className="px-4 py-2.5 font-medium">{g.nome}</td>
                    <td className="px-4 py-2.5 text-right">{num(g.requisicoes)}</td>
                    <td className="px-4 py-2.5 text-right">{num(g.cache_hits)}</td>
                    <td className="px-4 py-2.5 text-right">{usd(g.custo_micro)}</td>
                    <td className={`px-4 py-2.5 text-right ${g.erros ? "font-medium text-erro" : "text-suave"}`}>{num(g.erros)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
        <section aria-labelledby="t-prov" className="min-w-0">
          <h2 id="t-prov" className="mb-3 font-medium">
            Providers
          </h2>
          <ul className={`${cartao} divide-y divide-borda text-sm`}>
            {prov.data?.map((p) => (
              <li key={p.destino} className="flex items-center justify-between gap-3 px-4 py-2.5">
                <span className="truncate font-mono text-[13px]">{p.destino}</span>
                <Breaker estado={p.breaker} abertos={p.abertos} />
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}

// Vazao (flow) is the "floodgate": of everything that arrived, how much went through to the provider,
// how much came back from the cache without paying and how much was blocked (limit, budget, failure).
function Vazao({ r }: { r: Relatorio }) {
  const t = r.total;
  const barradas = t.erros;
  const cache = t.cache_hits;
  const provedor = Math.max(t.requisicoes - cache - barradas, 0);
  const partes = [
    { nome: "Paid to the provider", n: provedor, cor: "bg-acento" },
    { nome: "Answered by the cache", n: cache, cor: "bg-acento-claro" },
    { nome: "Blocked or failed", n: barradas, cor: "bg-erro" },
  ];
  return (
    <section aria-label="Flow in the last 24 hours">
      <p className="mb-1 text-sm text-suave">Last 24 hours</p>
      <p className="mb-4 text-3xl font-semibold tracking-tight">
        {num(t.requisicoes)} calls came in; <span className="text-acento">{num(provedor)} were paid for</span>.
      </p>
      <div
        className="flex h-3 gap-0.5 overflow-hidden rounded-full bg-borda"
        role="img"
        aria-label={partes.map((p) => `${p.nome}: ${p.n}`).join(", ")}
      >
        {partes.map((p) => p.n > 0 && <div key={p.nome} className={`${p.cor} transition-[flex-grow] duration-500 ease-out`} style={{ flexGrow: p.n }} />)}
      </div>
      <ul className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-sm text-suave">
        {partes.map((p) => (
          <li key={p.nome} className="flex items-center gap-2">
            <span className={`inline-block size-2.5 rounded-full ${p.cor}`} aria-hidden />
            {p.nome}: <span className="font-medium text-texto">{num(p.n)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

const corValor = { acento: "text-acento", alerta: "text-alerta", erro: "text-erro" } as const;
function Numero({ rotulo, valor, tom, largo }: { rotulo: string; valor: string; tom?: keyof typeof corValor; largo?: boolean }) {
  return (
    <div className={`${cartao} px-4 py-3 ${largo ? "col-span-2 sm:col-span-1" : ""}`}>
      <dt className="text-sm text-suave">{rotulo}</dt>
      <dd className={`mt-1 text-xl font-semibold tracking-tight ${tom ? corValor[tom] : ""}`}>{valor}</dd>
    </div>
  );
}

function Breaker({ estado, abertos }: { estado: string; abertos: number }) {
  const base = { closed: "Normal", "half-open": "Testing", open: "Breaker open" }[estado] ?? estado;
  const rotulo = abertos ? `${base} · ${abertos} ${abertos === 1 ? "client" : "clients"}` : base;
  return <Selo tom={estado === "closed" ? "ok" : estado === "open" ? "erro" : "alerta"}>{rotulo}</Selo>;
}
