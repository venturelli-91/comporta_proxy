import { useQuery } from "@tanstack/react-query";
import { api, num, usd, type Comparacao, type Modelo, type Relatorio } from "../api";
import { botao, cartao } from "../ui";
import { Estado } from "./Estado";

// Shadow: cada alias com sombra mostra o principal (que respondeu ao cliente) contra o
// candidato (chamado em segundo plano), para decidir uma migração com números.
export default function Shadow() {
  const uso = useQuery({ queryKey: ["uso"], queryFn: () => api<Relatorio>("/admin/uso"), refetchInterval: 5000 });
  const modelos = useQuery({ queryKey: ["modelos"], queryFn: () => api<Modelo[]>("/admin/modelos") });
  if (!uso.data || !modelos.data) return <Estado q={uso.data ? modelos : uso} />;
  const comSombra = modelos.data.filter((m) => m.shadow);
  if (comSombra.length === 0)
    return (
      <p className={`${cartao} max-w-xl p-5 text-sm text-suave`}>
        Nenhum modelo com shadow no <code className="font-mono">comporta.json</code>. Adicione <code className="font-mono">"shadow"</code> a um alias
        para comparar dois destinos sem afetar o usuário.
      </p>
    );

  return (
    <div className="space-y-6">
      <p className="max-w-2xl text-sm text-suave">
        O principal responde ao cliente; o candidato recebe a mesma chamada em segundo plano. A comparação ajuda a decidir uma migração de
        modelo ou provedor com números, sem risco.
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
                <span className="font-mono">{m.destinos[0]}</span> responde · <span className="font-mono">{m.shadow}</span> em segundo plano
              </p>
            </header>
            {!principal || !sombra ? (
              <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-4 text-sm text-suave">
                <span>Sem chamadas comparáveis nas últimas 24 horas.</span>
                <a className={botao} href="#playground">
                  Testar {m.alias} no Playground
                </a>
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead className="text-left text-suave">
                  <tr className="border-b border-borda">
                    <th className="px-5 py-2.5 font-medium">
                      <span className="sr-only">Métrica</span>
                    </th>
                    <th className="px-5 py-2.5 text-right font-medium">Principal</th>
                    <th className="px-5 py-2.5 text-right font-medium">Candidato</th>
                    <th className="px-5 py-2.5 text-right font-medium">Diferença</th>
                  </tr>
                </thead>
                <tbody>
                  <Linha nome="Latência média" a={principal} b={sombra} v={(c) => c.latencia_media_ms} f={(n) => `${num(n)} ms`} menorMelhor />
                  <Linha nome="Custo médio" a={principal} b={sombra} v={(c) => c.custo_medio_micro} f={usd} menorMelhor />
                  <Linha nome="Tamanho médio da resposta" a={principal} b={sombra} v={(c) => c.tamanho_medio} f={(n) => `${num(n)} bytes`} />
                  <Linha nome="Amostras" a={principal} b={sombra} v={(c) => c.requisicoes} f={num} semDiferenca />
                </tbody>
              </table>
            )}
          </section>
        );
      })}
    </div>
  );
}

// menorMelhor pinta a diferença: verde (teal) quando o candidato ganha, vermelho quando perde.
// Tamanho não é melhor nem pior, então fica neutro.
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
