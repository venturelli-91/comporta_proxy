import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, num, usd, type EstadoDestino, type Relatorio } from "../api";
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
      <p className="text-suave">
        Nenhuma chamada nas últimas 24 horas. Crie um cliente em <a className="text-acento underline" href="#clientes">Clientes e chaves</a> e
        mande a primeira pelo <a className="text-acento underline" href="#playground">Playground</a>.
      </p>
    );

  return (
    <div className="space-y-10">
      <Vazao r={uso.data} />

      <dl className="grid grid-cols-2 gap-x-8 gap-y-4 sm:grid-cols-3 lg:grid-cols-5">
        <Numero rotulo="Gasto" valor={usd(t.custo_micro)} />
        <Numero rotulo="Economia do cache" valor={usd(t.economia_micro)} />
        <Numero rotulo="Latência p95" valor={`${num(t.p95_ms)} ms`} />
        <Numero rotulo="Taxa de erro" valor={`${((100 * t.erros) / t.requisicoes).toFixed(1)}%`} alerta={t.erros > 0} />
        <Numero rotulo="Dados pessoais protegidos" valor={num(t.pii)} />
      </dl>

      <section aria-labelledby="g-minuto">
        <h2 id="g-minuto" className="mb-3 font-medium">Requisições por minuto</h2>
        <div className="h-56 rounded-lg border border-borda bg-superficie p-3">
          <ResponsiveContainer>
            <AreaChart data={uso.data.serie.map((p) => ({ ...p, hora: new Date(p.minuto * 1000).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" }) }))}>
              <CartesianGrid stroke="var(--borda)" vertical={false} />
              <XAxis dataKey="hora" stroke="var(--suave)" fontSize={12} />
              <YAxis stroke="var(--suave)" fontSize={12} allowDecimals={false} width={40} />
              <Tooltip contentStyle={{ background: "var(--superficie)", border: "1px solid var(--borda)" }} />
              <Area dataKey="requisicoes" name="Requisições" stroke="var(--acento)" fill="var(--acento-claro)" />
              <Area dataKey="erros" name="Erros" stroke="var(--erro)" fill="transparent" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </section>

      <div className="grid gap-10 lg:grid-cols-[2fr_1fr]">
        <section aria-labelledby="t-clientes">
          <h2 id="t-clientes" className="mb-3 font-medium">Por cliente</h2>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-suave">
                <tr className="border-b border-borda">
                  <th className="py-2 font-normal">Cliente</th>
                  <th className="py-2 text-right font-normal">Requisições</th>
                  <th className="py-2 text-right font-normal">Cache</th>
                  <th className="py-2 text-right font-normal">Gasto</th>
                  <th className="py-2 text-right font-normal">Erros</th>
                </tr>
              </thead>
              <tbody>
                {uso.data.por_cliente.map((g) => (
                  <tr key={g.nome} className="border-b border-borda last:border-0">
                    <td className="py-2">{g.nome}</td>
                    <td className="py-2 text-right">{num(g.requisicoes)}</td>
                    <td className="py-2 text-right">{num(g.cache_hits)}</td>
                    <td className="py-2 text-right">{usd(g.custo_micro)}</td>
                    <td className={`py-2 text-right ${g.erros ? "text-erro" : ""}`}>{num(g.erros)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
        <section aria-labelledby="t-prov">
          <h2 id="t-prov" className="mb-3 font-medium">Provedores</h2>
          <ul className="space-y-2 text-sm">
            {prov.data?.map((p) => (
              <li key={p.destino} className="flex items-center justify-between gap-3">
                <span className="truncate">{p.destino}</span>
                <Breaker estado={p.breaker} />
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}

// Vazao é a "comporta": de tudo que chegou, quanto passou para o provedor, quanto voltou do
// cache sem pagar e quanto foi barrado (limite, orçamento, falha).
function Vazao({ r }: { r: Relatorio }) {
  const t = r.total;
  const barradas = t.erros;
  const cache = t.cache_hits;
  const provedor = Math.max(t.requisicoes - cache - barradas, 0);
  const partes = [
    { nome: "Pagas ao provedor", n: provedor, cor: "bg-acento" },
    { nome: "Respondidas pelo cache", n: cache, cor: "bg-acento-claro" },
    { nome: "Barradas ou com falha", n: barradas, cor: "bg-erro" },
  ];
  return (
    <section aria-label="Vazão nas últimas 24 horas">
      <p className="mb-3 text-3xl font-semibold tracking-tight">
        {num(t.requisicoes)} chamadas chegaram; {num(provedor)} foram pagas.
      </p>
      <div className="flex h-4 overflow-hidden rounded-full bg-borda" role="img" aria-label={partes.map((p) => `${p.nome}: ${p.n}`).join(", ")}>
        {partes.map((p) => p.n > 0 && <div key={p.nome} className={p.cor} style={{ flexGrow: p.n }} />)}
      </div>
      <ul className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-sm text-suave">
        {partes.map((p) => (
          <li key={p.nome} className="flex items-center gap-2">
            <span className={`inline-block size-2.5 rounded-full ${p.cor}`} aria-hidden />
            {p.nome}: <span className="text-texto">{num(p.n)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Numero({ rotulo, valor, alerta }: { rotulo: string; valor: string; alerta?: boolean }) {
  return (
    <div>
      <dt className="text-sm text-suave">{rotulo}</dt>
      <dd className={`text-xl font-medium ${alerta ? "text-alerta" : ""}`}>{valor}</dd>
    </div>
  );
}

function Breaker({ estado }: { estado: string }) {
  const rotulo = { closed: "Normal", "half-open": "Testando", open: "Fora (breaker aberto)" }[estado] ?? estado;
  const cor = estado === "closed" ? "text-acento" : estado === "open" ? "text-erro" : "text-alerta";
  return <span className={`shrink-0 ${cor}`}>{rotulo}</span>;
}
