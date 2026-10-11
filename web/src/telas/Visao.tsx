import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, num, usd, type EstadoDestino, type Relatorio } from "../api";
import { botao, botaoSecundario, Selo } from "../ui";
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
      <div className="max-w-xl space-y-4 border-y border-borda py-6">
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
    <div className="space-y-12">
      <Vazao r={uso.data} />

      <dl className="grid grid-cols-2 border-t border-borda sm:grid-cols-3 lg:grid-cols-5 lg:border-b">
        <Numero rotulo="Spend" valor={usd(t.custo_micro)} />
        <Numero rotulo="Cache savings" valor={usd(t.economia_micro)} />
        <Numero rotulo="p95 latency" valor={`${num(t.p95_ms)} ms`} />
        <Numero rotulo="Error rate" valor={`${taxaErro.toFixed(1)}%`} tom={taxaErro >= 5 ? "erro" : t.erros > 0 ? "alerta" : undefined} />
        <Numero rotulo="Personal data protected" valor={num(t.pii)} tom={t.pii > 0 ? "acento" : undefined} />
      </dl>

      <section aria-labelledby="g-minuto">
        <h2 id="g-minuto" className={titulo}>
          Requests per minute
        </h2>
        <div className="h-56 border-b border-borda">
          <ResponsiveContainer>
            <AreaChart
              margin={{ top: 8, right: 4, bottom: 0, left: 0 }}
              data={uso.data.serie.map((p) => ({
                ...p,
                hora: new Date(p.minuto * 1000).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
              }))}
            >
              <CartesianGrid stroke="var(--borda)" strokeDasharray="2 4" vertical={false} />
              <XAxis dataKey="hora" stroke="var(--suave)" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis stroke="var(--suave)" fontSize={12} allowDecimals={false} width={32} tickLine={false} axisLine={false} />
              <Tooltip
                cursor={{ stroke: "var(--suave)", strokeDasharray: "3 3" }}
                contentStyle={{
                  background: "var(--superficie)",
                  border: "1px solid var(--borda)",
                  borderRadius: 3,
                  fontSize: 13,
                }}
                labelStyle={{ color: "var(--texto)", fontWeight: 600 }}
                itemStyle={{ color: "var(--texto)" }}
              />
              <Area dataKey="requisicoes" name="Requests" stroke="var(--acento)" strokeWidth={1.5} fill="var(--acento)" fillOpacity={0.08} />
              <Area dataKey="erros" name="Errors" stroke="var(--erro)" strokeWidth={1.5} fill="transparent" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </section>

      <div className="grid gap-12 lg:grid-cols-[2fr_1fr]">
        <section aria-labelledby="t-clientes" className="min-w-0">
          <h2 id="t-clientes" className={titulo}>
            By client
          </h2>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-suave">
                <tr className="border-y border-borda">
                  <th className="py-2 pr-4 font-medium">Client</th>
                  <th className="px-4 py-2 text-right font-medium">Requests</th>
                  <th className="px-4 py-2 text-right font-medium">Cache</th>
                  <th className="px-4 py-2 text-right font-medium">Spend</th>
                  <th className="py-2 pl-4 text-right font-medium">Errors</th>
                </tr>
              </thead>
              <tbody>
                {uso.data.por_cliente.map((g) => (
                  <tr key={g.nome} className="border-b border-borda hover:bg-realce">
                    <td className="py-2.5 pr-4 font-medium">{g.nome}</td>
                    <td className="px-4 py-2.5 text-right">{num(g.requisicoes)}</td>
                    <td className="px-4 py-2.5 text-right">{num(g.cache_hits)}</td>
                    <td className="px-4 py-2.5 text-right">{usd(g.custo_micro)}</td>
                    <td className={`py-2.5 pl-4 text-right ${g.erros ? "font-medium text-erro" : "text-suave"}`}>{num(g.erros)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
        <section aria-labelledby="t-prov" className="min-w-0">
          <h2 id="t-prov" className={titulo}>
            Providers
          </h2>
          <ul className="border-t border-borda text-sm">
            {prov.data?.map((p) => (
              <li key={p.destino} className="flex items-center justify-between gap-3 border-b border-borda py-2.5">
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

const titulo = "mb-3 text-[15px] font-semibold tracking-tight";

// Vazao (flow) is the floodgate drawn literally: everything that came in on the left, a channel that
// splits into what went through to the provider, what came back from the cache without paying and
// what was blocked (limit, budget, failure). Bar length is the share of the total.
function Vazao({ r }: { r: Relatorio }) {
  const t = r.total;
  const barradas = t.erros;
  const cache = t.cache_hits;
  const provedor = Math.max(t.requisicoes - cache - barradas, 0);
  const partes = [
    { nome: "Paid to the provider", n: provedor, cor: "bg-acento", texto: "" },
    { nome: "Answered by the cache", n: cache, cor: "bg-acento-claro", texto: "" },
    { nome: "Blocked or failed", n: barradas, cor: "bg-erro", texto: barradas ? "text-erro" : "" },
  ];
  const pct = (n: number) => (t.requisicoes ? (100 * n) / t.requisicoes : 0);
  return (
    <section aria-label="Flow in the last 24 hours" className="grid gap-6 md:grid-cols-[auto_1fr] md:gap-0">
      <div className="flex items-baseline gap-3 md:block md:self-center md:pr-8">
        <data value={t.requisicoes} className="block text-5xl leading-none font-semibold tracking-tight">
          {num(t.requisicoes)}
        </data>
        <span className="text-sm text-suave md:mt-2 md:block">calls came in</span>
      </div>
      {/* The channel: a branch per outlet; each row draws its piece of the 1px trunk so it ends at the last branch. */}
      <div className="relative md:self-center">
        <span className="absolute top-1/2 -left-8 hidden h-px w-8 bg-texto/60 md:block" aria-hidden />
        <ul>
          {partes.map((p) => (
            <li
              key={p.nome}
              className="relative grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1.5 py-2 pl-7 before:absolute before:top-1/2 before:left-0 before:h-px before:w-5 before:bg-texto/60 after:absolute after:inset-y-0 after:left-0 after:w-px after:bg-texto/60 first:after:top-1/2 last:after:bottom-1/2 sm:grid-cols-[11rem_1fr_3.5rem_3rem]"
            >
              <span className="text-sm text-suave">{p.nome}</span>
              <span className="col-span-2 row-start-2 h-3 bg-borda/60 sm:col-span-1 sm:row-start-auto">
                <span className={`block h-full ${p.cor} transition-[width] duration-200 ease-saida`} style={{ width: `${pct(p.n)}%` }} />
              </span>
              <data value={p.n} className={`row-start-1 text-right text-lg font-semibold ${p.texto} sm:row-start-auto`}>
                {num(p.n)}
              </data>
              <data value={pct(p.n)} className="hidden text-right text-sm text-suave sm:block">
                {pct(p.n).toFixed(0)}%
              </data>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

const corValor = { acento: "text-acento", alerta: "text-alerta", erro: "text-erro" } as const;
function Numero({ rotulo, valor, tom }: { rotulo: string; valor: string; tom?: keyof typeof corValor }) {
  return (
    <div className="flex flex-col-reverse justify-end gap-1.5 border-b border-borda py-4 pr-4 lg:border-b-0 lg:border-l lg:pl-5 lg:first:border-l-0 lg:first:pl-0">
      <dt className="text-sm text-suave">{rotulo}</dt>
      <dd className={`text-[1.625rem] leading-none font-semibold tracking-tight ${tom ? corValor[tom] : ""}`}>{valor}</dd>
    </div>
  );
}

function Breaker({ estado, abertos }: { estado: string; abertos: number }) {
  const base = { closed: "Normal", "half-open": "Testing", open: "Breaker open" }[estado] ?? estado;
  const rotulo = abertos ? `${base} · ${abertos} ${abertos === 1 ? "client" : "clients"}` : base;
  return <Selo tom={estado === "closed" ? "ok" : estado === "open" ? "erro" : "alerta"}>{rotulo}</Selo>;
}
