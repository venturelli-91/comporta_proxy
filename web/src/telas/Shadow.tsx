import { useQuery } from "@tanstack/react-query";
import { api, num, usd, type Comparacao, type Modelo, type Relatorio } from "../api";
import { Estado } from "./Estado";

// Shadow: cada alias com sombra mostra o principal (que respondeu ao cliente) contra o
// candidato (chamado em segundo plano), para decidir uma migração com números.
export default function Shadow() {
  const uso = useQuery({ queryKey: ["uso"], queryFn: () => api<Relatorio>("/admin/uso"), refetchInterval: 5000 });
  const modelos = useQuery({ queryKey: ["modelos"], queryFn: () => api<Modelo[]>("/admin/modelos") });
  if (!uso.data || !modelos.data) return <Estado q={uso.data ? modelos : uso} />;
  const comSombra = modelos.data.filter((m) => m.shadow);
  if (comSombra.length === 0)
    return <p className="text-suave">Nenhum modelo com shadow no comporta.json. Adicione "shadow" a um alias para comparar dois destinos.</p>;

  return (
    <div className="space-y-8">
      {comSombra.map((m) => {
        const linhas = uso.data.shadow.filter((c) => c.modelo === m.alias);
        const principal = linhas.find((c) => !c.shadow);
        const sombra = linhas.find((c) => c.shadow);
        return (
          <section key={m.alias} aria-labelledby={`s-${m.alias}`}>
            <h2 id={`s-${m.alias}`} className="mb-1 font-medium">{m.alias}</h2>
            <p className="mb-3 text-sm text-suave">
              {m.destinos[0]} responde; {m.shadow} é chamado em segundo plano.
            </p>
            {!principal || !sombra ? (
              <p className="text-sm text-suave">Sem chamadas comparáveis nas últimas 24 horas. Use o alias {m.alias} no Playground.</p>
            ) : (
              <table className="w-full max-w-2xl text-sm">
                <thead className="text-left text-suave">
                  <tr className="border-b border-borda">
                    <th className="py-2 font-normal"></th>
                    <th className="py-2 text-right font-normal">Principal</th>
                    <th className="py-2 text-right font-normal">Candidato</th>
                    <th className="py-2 text-right font-normal">Diferença</th>
                  </tr>
                </thead>
                <tbody>
                  <Linha nome="Latência média" a={principal} b={sombra} v={(c) => c.latencia_media_ms} f={(n) => `${num(n)} ms`} />
                  <Linha nome="Custo médio" a={principal} b={sombra} v={(c) => c.custo_medio_micro} f={usd} />
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

function Linha(p: { nome: string; a: Comparacao; b: Comparacao; v: (c: Comparacao) => number; f: (n: number) => string; semDiferenca?: boolean }) {
  const a = p.v(p.a);
  const b = p.v(p.b);
  const dif = a ? Math.round((100 * (b - a)) / a) : 0;
  return (
    <tr className="border-b border-borda last:border-0">
      <td className="py-2">{p.nome}</td>
      <td className="py-2 text-right">{p.f(a)}</td>
      <td className="py-2 text-right">{p.f(b)}</td>
      <td className="py-2 text-right text-suave">{p.semDiferenca ? "" : `${dif > 0 ? "+" : ""}${dif}%`}</td>
    </tr>
  );
}
