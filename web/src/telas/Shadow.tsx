import { useQuery } from "@tanstack/react-query";
import { api, num, usd, type Comparacao, type Modelo, type Relatorio } from "../api";
import { botao, cartao } from "../ui";
import { Estado } from "./Estado";

// Shadow: each alias with a shadow shows the primary (which answered the client) against the
// candidate (called in the background), to decide a migration with numbers.
export default function Shadow() {
  const uso = useQuery({ queryKey: ["uso"], queryFn: () => api<Relatorio>("/admin/uso"), refetchInterval: 5000 });
  const modelos = useQuery({ queryKey: ["modelos"], queryFn: () => api<Modelo[]>("/admin/modelos") });
  if (!uso.data || !modelos.data) return <Estado q={uso.data ? modelos : uso} />;
  const comSombra = modelos.data.filter((m) => m.shadow);
  if (comSombra.length === 0)
    return (
      <p className={`${cartao} max-w-xl p-5 text-sm text-suave`}>
        No model has a shadow in <code className="font-mono">comporta.json</code>. Add <code className="font-mono">"shadow"</code> to an alias
        to compare two destinations without affecting the user.
      </p>
    );

  return (
    <div className="space-y-6">
      <p className="max-w-2xl text-sm text-suave">
        The primary answers the client; the candidate gets the same call in the background. The comparison helps decide a model or
        provider migration with numbers, without risk.
      </p>
      {comSombra.map((m) => {
        const linhas = uso.data.shadow.filter((c) => c.modelo === m.alias);
        const principal = linhas.find((c) => !c.shadow);
        const sombra = linhas.find((c) => c.shadow);
        return (
          <section key={m.alias} aria-labelledby={`s-${m.alias}`} className={`${cartao} max-w-3xl`}>
            <header className="flex flex-wrap items-baseline justify-between gap-2 border-b border-borda px-5 py-3">
              <h2 id={`s-${m.alias}`} className="font-medium">
                {m.alias}
              </h2>
              <p className="text-sm text-suave">
                <span className="font-mono">{m.destinos[0]}</span> answers · <span className="font-mono">{m.shadow}</span> in the background
              </p>
            </header>
            {!principal || !sombra ? (
              <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-4 text-sm text-suave">
                <span>No comparable calls in the last 24 hours.</span>
                <a className={botao} href="#playground">
                  Try {m.alias} in the Playground
                </a>
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead className="text-left text-suave">
                  <tr className="border-b border-borda">
                    <th className="px-5 py-2.5 font-medium">
                      <span className="sr-only">Metric</span>
                    </th>
                    <th className="px-5 py-2.5 text-right font-medium">Primary</th>
                    <th className="px-5 py-2.5 text-right font-medium">Candidate</th>
                    <th className="px-5 py-2.5 text-right font-medium">Difference</th>
                  </tr>
                </thead>
                <tbody>
                  <Linha nome="Average latency" a={principal} b={sombra} v={(c) => c.latencia_media_ms} f={(n) => `${num(n)} ms`} menorMelhor />
                  <Linha nome="Average cost" a={principal} b={sombra} v={(c) => c.custo_medio_micro} f={usd} menorMelhor />
                  <Linha nome="Average response size" a={principal} b={sombra} v={(c) => c.tamanho_medio} f={(n) => `${num(n)} bytes`} />
                  <Linha nome="Samples" a={principal} b={sombra} v={(c) => c.requisicoes} f={num} semDiferenca />
                </tbody>
              </table>
            )}
          </section>
        );
      })}
    </div>
  );
}

// menorMelhor (lower is better) colors the difference: teal when the candidate wins, red when it loses.
// Size is neither better nor worse, so it stays neutral.
function Linha(p: {
  nome: string;
  a: Comparacao;
  b: Comparacao;
  v: (c: Comparacao) => number;
  f: (n: number) => string;
  semDiferenca?: boolean;
  menorMelhor?: boolean;
}) {
  const a = p.v(p.a);
  const b = p.v(p.b);
  const dif = a ? Math.round((100 * (b - a)) / a) : 0;
  const cor = !p.menorMelhor || dif === 0 ? "text-suave" : dif < 0 ? "text-acento" : "text-erro";
  return (
    <tr className="border-b border-borda last:border-0 hover:bg-realce">
      <td className="px-5 py-2.5">{p.nome}</td>
      <td className="px-5 py-2.5 text-right">{p.f(a)}</td>
      <td className="px-5 py-2.5 text-right">{p.f(b)}</td>
      <td className={`px-5 py-2.5 text-right font-medium ${cor}`}>{p.semDiferenca ? "" : `${dif > 0 ? "+" : ""}${dif}%`}</td>
    </tr>
  );
}
